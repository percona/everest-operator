package webhooks

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	everestv1alpha1 "github.com/percona/everest-operator/api/v1alpha1"
)

// SetupDataImportJobWebhookWithManager sets up the webhook with the manager.
func SetupDataImportJobWebhookWithManager(mgr manager.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
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

func (v *DataImportJobValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	dij, ok := obj.(*everestv1alpha1.DataImportJob)
	if !ok {
		return nil, fmt.Errorf("expected a DataImportJob, got %T", obj)
	}
	return nil, v.validate(ctx, dij)
}

func (v *DataImportJobValidator) ValidateUpdate(ctx context.Context, _, newObj runtime.Object) (admission.Warnings, error) {
	dij, ok := newObj.(*everestv1alpha1.DataImportJob)
	if !ok {
		return nil, fmt.Errorf("expected a DataImportJob, got %T", newObj)
	}
	return nil, v.validate(ctx, dij)
}

func (v *DataImportJobValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	dij, ok := obj.(*everestv1alpha1.DataImportJob)
	if !ok {
		return nil, fmt.Errorf("expected a DataImportJob, got %T", obj)
	}
	return nil, v.validate(ctx, dij)
}

func (v *DataImportJobValidator) validate(
	ctx context.Context,
	dij *everestv1alpha1.DataImportJob,
) error {
	dataimporter := everestv1alpha1.DataImporter{}
	if err := v.Client.Get(ctx, client.ObjectKeyFromObject(&dataimporter), &dataimporter); err != nil {
		return fmt.Errorf("failed to get DataImporter: %w", err)
	}
	return dataimporter.Spec.Config.ValidateParams(dij.Spec.Params)
}
