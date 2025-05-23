package webhooks

import (
	"context"
	"fmt"

	"github.com/AlekSi/pointer"
	everestv1alpha1 "github.com/percona/everest-operator/api/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// SetupDatabaseClusterWebhookWithManager sets up the webhook with the manager.
func SetupDatabaseClusterWebhookWithManager(mgr manager.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&everestv1alpha1.DatabaseCluster{}).
		WithValidator(&DatabaseClusterValidator{
			Client: mgr.GetClient(),
		}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-everest-percona-com-v1alpha1-databasecluster,mutating=false,failurePolicy=fail,sideEffects=None,groups=everest.percona.com,resources=databaseclusters,verbs=create;update,versions=v1alpha1,name=vdatabasecluster-v1alpha1.everest.percona.com,admissionReviewVersions=v1

// DatabaseClusterValidator validates the DatabaseCluster resource.
type DatabaseClusterValidator struct {
	Client client.Client
}

// ValidateCreate validates the creation of a DatabaseCluster.
func (v *DatabaseClusterValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	db, ok := obj.(*everestv1alpha1.DatabaseCluster)
	if !ok {
		return nil, fmt.Errorf("expected a DatabaseCluster, got %T", obj)
	}
	return nil, v.validate(ctx, db)
}

// ValidateUpdate validates the update of a DatabaseCluster.
func (v *DatabaseClusterValidator) ValidateUpdate(ctx context.Context, _, newObj runtime.Object) (admission.Warnings, error) {
	// updated, ok := newObj.(*everestv1alpha1.DatabaseCluster)
	// if !ok {
	// 	return nil, fmt.Errorf("expected a DatabaseCluster (new), got %T", newObj)
	// }

	// old, ok := newObj.(*everestv1alpha1.DatabaseCluster)
	// if !ok {
	// 	return nil, fmt.Errorf("expected a DatabaseCluster (old), got %T", newObj)
	// }

	// updatedImportTpl := pointer.Get(updated.Spec.DataSource).DataImport
	// oldImportTpl := pointer.Get(old.Spec.DataSource).DataImport

	// if !updatedImportTpl.Equals(pointer.Get(oldImportTpl)) {
	// 	return nil, errors.New(".spec.dataSource.dataImport is immutable")
	// }
	return nil, nil
}

// ValidateDelete validates the deletion of a DatabaseCluster.
func (v *DatabaseClusterValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

func (v *DatabaseClusterValidator) validate(
	ctx context.Context,
	db *everestv1alpha1.DatabaseCluster,
) error {
	dataImport := pointer.Get(db.Spec.DataSource).DataImport
	if dataImport == nil {
		return nil
	}

	dataimporter := &everestv1alpha1.DataImporter{}
	if err := v.Client.Get(ctx, types.NamespacedName{
		Name: dataImport.DataImporterName,
	}, dataimporter); err != nil {
		return fmt.Errorf("failed to get DataImporter: %w", err)
	}

	// Validate that the DataImporter supports the specified engine.
	engineType := db.Spec.Engine.Type
	if !dataimporter.Spec.SupportedEngines.Has(engineType) {
		return fmt.Errorf("data importer %s does not support engine %s", dataImport.DataImporterName, engineType)
	}

	// Validate the params against the schema of the DataImporter
	return dataimporter.Spec.Config.ValidateParams(dataImport.Params)
}
