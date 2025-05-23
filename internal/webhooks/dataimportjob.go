package webhooks

import (
	"context"
	"errors"
	"fmt"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	everestv1alpha1 "github.com/percona/everest-operator/api/v1alpha1"
)

// SetupDataImportJobWebhookWithManager sets up the webhook with the manager.
func SetupDataImportJobWebhookWithManager(mgr manager.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&everestv1alpha1.DataImportJob{}).
		WithValidator(&DataImportJobValidator{
			Client: mgr.GetClient(),
		}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-everest-percona-com-v1alpha1-dataimportjob,mutating=false,failurePolicy=fail,sideEffects=None,groups=everest.percona.com,resources=dataimportjobs,verbs=create;update,versions=v1alpha1,name=vdataimportjob-v1alpha1.everest.percona.com,admissionReviewVersions=v1

// DataImportJobValidator validates the DataImportJob resource.
type DataImportJobValidator struct {
	Client client.Client
}

// ValidateCreate validates the creation of a DataImportJob.
func (v *DataImportJobValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	dij, ok := obj.(*everestv1alpha1.DataImportJob)
	if !ok {
		return nil, fmt.Errorf("expected a DataImportJob, got %T", obj)
	}
	return nil, v.validate(ctx, dij)
}

// ValidateUpdate validates the update of a DataImportJob.
func (v *DataImportJobValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	updated, ok := newObj.(*everestv1alpha1.DataImportJob)
	if !ok {
		return nil, fmt.Errorf("expected a DataImportJob (new), got %T", newObj)
	}

	old, ok := oldObj.(*everestv1alpha1.DataImportJob)
	if !ok {
		return nil, fmt.Errorf("expected a DataImportJob (old), got %T", oldObj)
	}
	if !updated.Spec.Equals(old.Spec) {
		return nil, errors.New(".spec is immutable")
	}
	return nil, nil
}

func (v *DataImportJobValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

func (v *DataImportJobValidator) validate(
	ctx context.Context,
	dij *everestv1alpha1.DataImportJob,
) error {
	dataimporter := everestv1alpha1.DataImporter{}
	if err := v.Client.Get(ctx, types.NamespacedName{
		Name: dij.Spec.DataImporterName,
	}, &dataimporter); err != nil {
		return fmt.Errorf("failed to get DataImporter: %w", err)
	}

	db := everestv1alpha1.DatabaseCluster{}
	if err := v.Client.Get(ctx, types.NamespacedName{
		Name:      dij.Spec.TargetClusterName,
		Namespace: dij.GetNamespace(),
	}, &db); err != nil {
		if k8serrors.IsNotFound(err) {
			return fmt.Errorf("target cluster %s not found: %w", dij.Spec.TargetClusterName, err)
		}
		return fmt.Errorf("failed to get DatabaseCluster: %w", err)
	}

	// Validate that the DataImporter supports the specified engine.
	engineType := db.Spec.Engine.Type
	if !dataimporter.Spec.SupportedEngines.Has(engineType) {
		return fmt.Errorf("data importer %s does not support engine %s", dij.Spec.DataImporterName, engineType)
	}

	// Validate the params against the schema of the DataImporter
	return dataimporter.Spec.Config.ValidateParams(dij.Spec.Params)
}
