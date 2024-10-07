// everest-operator
// Copyright (C) 2022 Percona LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package main ...
//
//nolint:goimports
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/utils/strings/slices"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	everestv1alpha1 "github.com/percona/everest-operator/api/v1alpha1"
	"github.com/percona/everest-operator/controllers"
	"github.com/percona/everest-operator/controllers/common"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

const (
	systemNamespaceEnvVar     = "SYSTEM_NAMESPACE"
	monitoringNamespaceEnvVar = "MONITORING_NAMESPACE"
	dbNamespacesEnvVar        = "DB_NAMESPACES"
)

// Config contains the configuration for Everest Operator.
type Config struct {
	// MetricsAddr is the address the metric endpoint binds to.
	MetricsAddr string
	// EnableLeaderElection enables leader election for controller manager.
	EnableLeaderElection bool
	// LeaderElectionID is the name of the leader election ID.
	LeaderElectionID string
	// ProbeAddr is the address the health probe endpoint binds to.
	ProbeAddr string
	// WatchAllNamespaces is set to true if all namespaces should be watched.
	// This setting is ignored if DBNamespaces is set.
	WatchAllNamespaces bool
	// DBNamespaces is the namespaces to watch for DB resources.
	DBNamespaces string
	// MonitoringNamespace is the namespace where the monitoring resources are.
	MonitoringNamespace string
	// SystemNamespace is the namespace where the operator is running.
	SystemNamespace string
}

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(everestv1alpha1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	cfg, err := parseConfig()
	if err != nil {
		setupLog.Error(err, "unable to parse config")
		os.Exit(1)
	}
	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	dbNamespaces := []string{}
	slices.Filter(dbNamespaces, strings.Split(cfg.DBNamespaces, ","), func(s string) bool {
		return s != ""
	})
	defaultCache := cache.Options{}
	if len(dbNamespaces) > 0 {
		defaultCache.DefaultNamespaces = make(map[string]cache.Config)
		defaultCache.DefaultNamespaces[cfg.SystemNamespace] = cache.Config{}
		defaultCache.DefaultNamespaces[cfg.MonitoringNamespace] = cache.Config{}
		for _, ns := range dbNamespaces {
			defaultCache.DefaultNamespaces[ns] = cache.Config{}
		}
	}

	l := zap.New(zap.UseFlagOptions(&opts))
	ctrl.SetLogger(l)
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme: scheme,
		Metrics: metricsserver.Options{
			BindAddress: cfg.MetricsAddr,
		},
		HealthProbeBindAddress: cfg.ProbeAddr,
		LeaderElection:         cfg.EnableLeaderElection,
		LeaderElectionID:       cfg.LeaderElectionID,
		Cache:                  defaultCache,
		// LeaderElectionReleaseOnCancel defines if the leader should step down voluntarily
		// when the Manager ends. This requires the binary to immediately end when the
		// Manager is stopped, otherwise, this setting is unsafe. Setting this significantly
		// speeds up voluntary leader transitions as the new leader don't have to wait
		// LeaseDuration time first.
		//
		// In the default scaffold provided, the program ends immediately after
		// the manager stops, so would be fine to enable this option. However,
		// if you are doing or is intended to do any operation such as perform cleanups
		// after the manager stops then its usage might be unsafe.
		// LeaderElectionReleaseOnCancel: true,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Configure the default namespace filter.
	enableNsFilter := len(dbNamespaces) == 0 && !cfg.WatchAllNamespaces
	common.DefaultNamespaceFilter.Enabled = enableNsFilter
	common.DefaultNamespaceFilter.C = mgr.GetClient()
	common.DefaultNamespaceFilter.Log = ctrl.Log.WithName("namespace-filter")
	common.DefaultNamespaceFilter.AllowNamespaces = []string{cfg.SystemNamespace, cfg.MonitoringNamespace}
	common.DefaultNamespaceFilter.MatchLabels = map[string]string{
		common.LabelKubernetesManagedBy: common.Everest,
	}

	// Ensure specified DB namespaces exist.
	namespace := &unstructured.Unstructured{}
	namespace.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "",
		Kind:    "Namespace",
		Version: "v1",
	})
	for _, ns := range dbNamespaces {
		if err := mgr.GetClient().Get(context.Background(), types.NamespacedName{Name: ns}, namespace); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "DatabaseCluster")
			os.Exit(1)
		}
	}
	// Initialise the controllers.
	if err = (&controllers.DatabaseClusterReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr, cfg.SystemNamespace, cfg.MonitoringNamespace); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "DatabaseCluster")
		os.Exit(1)
	}
	if err = (&controllers.DatabaseEngineReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr, []string{}); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "DatabaseEngine")
		os.Exit(1)
	}
	if err = (&controllers.DatabaseClusterRestoreReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr, cfg.SystemNamespace); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "DatabaseClusterRestore")
		os.Exit(1)
	}
	if err = (&controllers.DatabaseClusterBackupReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr, cfg.SystemNamespace); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "DatabaseClusterBackup")
		os.Exit(1)
	}
	if err = (&controllers.BackupStorageReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr, cfg.SystemNamespace); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "BackupStorage")
		os.Exit(1)
	}
	if err = (&controllers.MonitoringConfigReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr, cfg.MonitoringNamespace); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "MonitoringConfig")
		os.Exit(1)
	}
	//+kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}

func parseConfig() (Config, error) {
	cfg := Config{}

	systemNamespace := os.Getenv(systemNamespaceEnvVar)
	monitoringNamespace := os.Getenv(monitoringNamespaceEnvVar)
	dbNamespacesString := os.Getenv(dbNamespacesEnvVar)

	flag.StringVar(&cfg.MetricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&cfg.ProbeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&cfg.EnableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&cfg.LeaderElectionID, "leader-election-id", "9094838c.percona.com",
		"The name of the leader election ID.")
	flag.BoolVar(&cfg.WatchAllNamespaces, "watch-all-namespaces", false, "If set, watch all namespaces."+
		"This setting is ignored if db-namespaces is set.")
	flag.StringVar(&cfg.DBNamespaces, "db-namespaces", dbNamespacesString, "The namespaces to watch for DB resources."+
		"Defaults to the value of the DB_NAMESPACES environment variable. If set, watch-all-namespaces is ignored.")
	flag.StringVar(&cfg.SystemNamespace, "system-namespace", systemNamespace, "The namespace where the operator is running."+
		"Defaults to the value of the SYSTEM_NAMESPACE environment variable.")
	flag.StringVar(&cfg.MonitoringNamespace, "monitoring-namespace", monitoringNamespace, "The namespace where the monitoring resources are."+
		"Defaults to the value of the MONITORING_NAMESPACE environment variable.")
	flag.Parse()

	if cfg.SystemNamespace == "" {
		return cfg, fmt.Errorf("%s or --system-namespace must be set", systemNamespaceEnvVar)
	}
	if cfg.MonitoringNamespace == "" {
		return cfg, fmt.Errorf("%s or --monitoring-namespace must be set", monitoringNamespaceEnvVar)
	}
	return cfg, nil
}
