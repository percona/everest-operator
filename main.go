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
	"os"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
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
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

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

	dbNamespaces := strings.Split(cfg.DBNamespace, ",")
	defaultCache := cache.Options{}
	if len(dbNamespaces) > 0 {
		defaultCache.DefaultNamespaces = make(map[string]cache.Config)
		defaultCache.DefaultNamespaces[cfg.SystemNamespace] = cache.Config{}
		defaultCache.DefaultNamespaces[cfg.MonitoringNamespace] = cache.Config{}
		for _, ns := range dbNamespaces {
			defaultCache.DefaultNamespaces[ns] = cache.Config{}
		}
	}

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))
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
	for _, ns := range dbNamespaces {
		namespace := &corev1.Namespace{}
		if err := mgr.GetClient().Get(context.Background(), types.NamespacedName{Name: ns}, namespace); err != nil {
			setupLog.Error(err, "unable to create controller", "controller", "DatabaseCluster")
			os.Exit(1)
		}
	}
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
