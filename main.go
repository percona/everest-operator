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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	everestv1alpha1 "github.com/percona/everest-operator/api/v1alpha1"
	"github.com/percona/everest-operator/controllers"
	"github.com/percona/everest-operator/controllers/common"
	"github.com/percona/everest-operator/pkg/predicates"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

const (
	systemNamespaceEnvVar     = "SYSTEM_NAMESPACE"
	monitoringNamespaceEnvVar = "MONITORING_NAMESPACE"
	dbNamespacesEnvVar        = "DB_NAMESPACES"
	podNameEnvVar             = "POD_NAME"
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
	// DBNamespaces is the namespaces to watch for DB resources.
	// If set, NamespaceLabels is ignored.
	DBNamespaces string
	// MonitoringNamespace is the namespace where the monitoring resources are.
	MonitoringNamespace string
	// SystemNamespace is the namespace where the operator is running.
	SystemNamespace string
	// Name of the pod.
	PodName string
	// If set, watches only those namespaces that have the specified labels.
	// This setting is ignored if DBNamespaces is set.
	NamespaceLabels map[string]string
}

var cfg = &Config{}

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(everestv1alpha1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func main() {
	err := parseConfig()
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
	if s := cfg.DBNamespaces; s != "" {
		dbNamespaces = strings.Split(s, ",")
	}
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

	// We filter namespaces only if no DBNamespaces are defined.
	if len(dbNamespaces) == 0 {
		common.DefaultNamespaceFilter = &predicates.NamespaceFilter{
			MatchLabels: cfg.NamespaceLabels,
			GetNamespace: func(ctx context.Context, name string) (*corev1.Namespace, error) {
				namespace := &corev1.Namespace{}
				if err := mgr.GetClient().Get(ctx, types.NamespacedName{Name: name}, namespace); err != nil {
					return nil, err
				}
				return namespace, nil
			},
		}
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
	podRef := corev1.ObjectReference{Name: cfg.PodName, Namespace: cfg.SystemNamespace}
	// Initialise the controllers.
	if err = (&controllers.DatabaseClusterReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "DatabaseCluster")
		os.Exit(1)
	}
	if err = (&controllers.DatabaseEngineReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr, podRef, dbNamespaces); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "DatabaseEngine")
		os.Exit(1)
	}
	if err = (&controllers.DatabaseClusterRestoreReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "DatabaseClusterRestore")
		os.Exit(1)
	}
	if err = (&controllers.DatabaseClusterBackupReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "DatabaseClusterBackup")
		os.Exit(1)
	}
	if err = (&controllers.BackupStorageReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
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

func parseConfig() error {
	systemNamespace := os.Getenv(systemNamespaceEnvVar)
	monitoringNamespace := os.Getenv(monitoringNamespaceEnvVar)
	dbNamespacesString := os.Getenv(dbNamespacesEnvVar)
	podName := os.Getenv(podNameEnvVar)
	cfg.PodName = podName
	defaultNamespaceLabelFilter := fmt.Sprintf("%s=%s", common.LabelKubernetesManagedBy, common.Everest)
	var namespaceLabelFilter string
	flag.StringVar(&cfg.MetricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&cfg.ProbeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&cfg.EnableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&cfg.LeaderElectionID, "leader-election-id", "9094838c.percona.com",
		"The name of the leader election ID.")
	flag.StringVar(&namespaceLabelFilter, "namespace-label-filter", defaultNamespaceLabelFilter,
		"If set, reconciles objects only from those namespaces that have the specified label. "+
			"This setting is ignored if db-namespaces is set. "+
			fmt.Sprintf("Default is `%s`", defaultNamespaceLabelFilter))
	flag.StringVar(&cfg.DBNamespaces, "db-namespaces", dbNamespacesString, "The namespaces to watch for DB resources."+
		"Defaults to the value of the DB_NAMESPACES environment variable. If set, watch-namespace-labels is ignored.")
	flag.StringVar(&cfg.SystemNamespace, "system-namespace", systemNamespace, "The namespace where the operator is running."+
		"Defaults to the value of the SYSTEM_NAMESPACE environment variable.")
	flag.StringVar(&cfg.MonitoringNamespace, "monitoring-namespace", monitoringNamespace, "The namespace where the monitoring resources are."+
		"Defaults to the value of the MONITORING_NAMESPACE environment variable.")
	flag.Parse()

	if cfg.SystemNamespace == "" {
		return fmt.Errorf("%s or --system-namespace must be set", systemNamespaceEnvVar)
	}
	if cfg.MonitoringNamespace == "" {
		return fmt.Errorf("%s or --monitoring-namespace must be set", monitoringNamespaceEnvVar)
	}

	cfg.NamespaceLabels = make(map[string]string)
	labels := strings.Split(namespaceLabelFilter, ",")
	for _, label := range labels {
		parts := strings.Split(label, "=")
		if len(parts) != 2 { //nolint:mnd
			return fmt.Errorf("invalid label filter: %s", label)
		}
		cfg.NamespaceLabels[parts[0]] = parts[1]
	}
	if cfg.DBNamespaces == "" && len(cfg.NamespaceLabels) == 0 {
		return fmt.Errorf("either %s or --namespace-label-filter must be set", dbNamespacesEnvVar)
	}
	return nil
}
