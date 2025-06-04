package webhooks

import (
	"context"
	"fmt"

	"github.com/AlekSi/pointer"
	everestv1alpha1 "github.com/percona/everest-operator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
)

var logger = logf.Log.WithName("dbwebhook")

// +kubebuilder:webhook:path=/mutate-everest-percona-com-v1alpha1-databasecluster,mutating=true,failurePolicy=fail,sideEffects=None,groups=everest.percona.com,resources=databaseclusters,verbs=create;update,versions=v1alpha1,name=mdatabasecluster-v1alpha1.everest.percona.com,admissionReviewVersions=v1
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=create;update;get;list
// +kubebuilder:rbac:groups=everest.percona.com,resources=databaseclusters,verbs=get;list;watch;update

// DatabaseClusterDefaulter is a webhook that sets default values for DatabaseCluster resources.
type DatabaseClusterDefaulter struct {
	// Client is the Kubernetes client used to interact with the cluster.
	Client client.Client
}

func (d *DatabaseClusterDefaulter) Default(ctx context.Context, obj runtime.Object) error {
	db, ok := obj.(*everestv1alpha1.DatabaseCluster)
	if !ok {
		return fmt.Errorf("expected a DatabaseCluster object but got %T", obj)
	}
	importTpl := pointer.Get(db.Spec.DataSource).DataImport
	return handleS3CredentialsSecret(ctx, d.Client, db.GetNamespace(), importTpl)
}
