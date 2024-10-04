package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/percona/everest-operator/controllers/common"
)

const (
	systemNamespaceEnvVar     = "SYSTEM_NAMESPACE"
	monitoringNamespaceEnvVar = "MONITORING_NAMESPACE"
	dbNamespacesEnvVar        = "DB_NAMESPACES"
)

// Config contains the configuration for Everest Operator.
type Config struct {
	MetricsAddr          string
	EnableLeaderElection bool
	LeaderElectionID     string
	ProbeAddr            string
	WatchMode            string
	DBNamespace          string
	MonitoringNamespace  string
	SystemNamespace      string
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
	flag.StringVar(&cfg.WatchMode, common.WatchModeCluster, "cluster", "The mode to watch resources in."+
		"Can be 'restricted' or 'cluster'. If unspecified, defaults to 'cluster'")
	flag.StringVar(&cfg.DBNamespace, "db-namespaces", dbNamespacesString, "The namespaces to watch for DB resources."+
		"Defaults to the value of the DB_NAMESPACES environment variable. If set, watch-mode is ignored.")
	flag.StringVar(&cfg.SystemNamespace, "system-namespace", systemNamespace, "The namespace where the operator is running."+
		"Defaults to the value of the SYSTEM_NAMESPACE environment variable.")
	flag.StringVar(&cfg.MonitoringNamespace, "monitoring-namespace", monitoringNamespace, "The namespace where the monitoring resources are."+
		"Defaults to the value of the MONITORING_NAMESPACE environment variable.")
	flag.Parse()

	if cfg.SystemNamespace == "" {
		return cfg, fmt.Errorf("%s or --systen-namespace must be set", systemNamespaceEnvVar)
	}
	if cfg.MonitoringNamespace == "" {
		return cfg, fmt.Errorf("%s or --monitoring-namespace must be set", monitoringNamespaceEnvVar)
	}
	return cfg, nil
}
