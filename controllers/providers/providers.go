package providers

import (
	everestv1alpha1 "github.com/percona/everest-operator/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type ProviderOptions struct {
	C            client.Client
	DB           *everestv1alpha1.DatabaseCluster
	DBEngine     *everestv1alpha1.DatabaseEngine
	SystemNs     string
	MonitoringNs string
}
