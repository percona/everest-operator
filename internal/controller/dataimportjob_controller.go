package controllers

import (
	"context"
	"errors"
	"fmt"

	everestv1alpha1 "github.com/percona/everest-operator/api/v1alpha1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type DataImportJobReconciler struct {
	Client client.Client
	Scheme *runtime.Scheme
}

func (r *DataImportJobReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named("DataImportJob").
		For(&everestv1alpha1.DataImportJob{}).
		Owns(&batchv1.Job{}).
		Owns(&corev1.Secret{}).
		Complete(r)
}

func (r *DataImportJobReconciler) Reconcile(
	ctx context.Context,
	req ctrl.Request) (rr ctrl.Result, rerr error) {
	diJob := &everestv1alpha1.DataImportJob{}
	if err := r.Client.Get(ctx, req.NamespacedName, diJob); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	l := log.FromContext(ctx)
	l.Info("Reconciling", "name", diJob.GetName())

	// Sync status on finishing reconciliation.
	defer func() {
		if updErr := r.Client.Status().Update(ctx, diJob); updErr != nil {
			l.Error(updErr, "Failed to update data import job status")
			rerr = errors.Join(rerr, updErr)
		}
	}()

	// Get the referenced data importer.
	di := &everestv1alpha1.DataImporter{}
	if err := r.Client.Get(ctx, client.ObjectKey{
		Name: diJob.Spec.DataImporterRef.Name,
	}, di); err != nil {
		diJob.Status.Phase = everestv1alpha1.DataImportJobPhaseError
		diJob.Status.Message = err.Error()
		return ctrl.Result{}, err
	}

	// Get the target database cluster.
	db := &everestv1alpha1.DatabaseCluster{}
	if err := r.Client.Get(ctx, client.ObjectKey{
		Name:      diJob.Spec.TargetClusterRef.Name,
		Namespace: diJob.GetNamespace(),
	}, db); err != nil {
		diJob.Status.Phase = everestv1alpha1.DataImportJobPhaseError
		diJob.Status.Message = fmt.Sprintf("failed to get database cluster: %w", err)
		return ctrl.Result{}, err
	}

	// Ensure Secret for database credentials.
	if err := r.ensureDBSecret(ctx, db); err != nil {
		diJob.Status.Phase = everestv1alpha1.DataImportJobPhaseError
		diJob.Status.Message = fmt.Sprintf("failed to ensure database secret: %w", err)
		return ctrl.Result{}, err
	}

	// Ensure import job.
	if err := r.ensureImportJob(ctx, diJob, di, db); err != nil {
		diJob.Status.Phase = everestv1alpha1.DataImportJobPhaseError
		diJob.Status.Message = fmt.Sprintf("failed to ensure import job: %w", err)
		return ctrl.Result{}, err
	}

	// Reconcile status.

	return ctrl.Result{}, nil
}

func (r *DataImportJobReconciler) ensureDBSecret(ctx context.Context, db *everestv1alpha1.DatabaseCluster) error {
	return nil
}

func (r *DataImportJobReconciler) ensureImportJob(
	ctx context.Context,
	diJob *everestv1alpha1.DataImportJob,
	di *everestv1alpha1.DataImporter,
	db *everestv1alpha1.DatabaseCluster,
) error {
	return nil
}
