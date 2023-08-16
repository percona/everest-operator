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

package scheduledbackup

import (
	"context"

	pxcv1 "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	everestv1alpha1 "github.com/percona/everest-operator/api/v1alpha1"
	"github.com/percona/everest-operator/controllers"
)

// DatabaseClusterPXCBackupReconciler reconciles a DatabaseClusterBackup object.
type DatabaseClusterPXCBackupReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=everest.percona.com,resources=databaseclusterbackups,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=everest.percona.com,resources=databaseclusterbackups/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=everest.percona.com,resources=databaseclusterbackups/finalizers,verbs=update
// +kubebuilder:rbac:groups=pgv2.percona.com,resources=perconapgrestores,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// Modify the Reconcile function to compare the state specified by
// the DatabaseClusterBackup object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.1/pkg/reconcile
func (r *DatabaseClusterPXCBackupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling", "request", req)

	backup := &everestv1alpha1.DatabaseClusterBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Name,
			Namespace: req.Namespace,
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       controllers.DatabaseClusterBackupKind,
			APIVersion: controllers.DatabaseClusterBackupAPI,
		},
	}
	err := r.Get(ctx, req.NamespacedName, backup)
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			return reconcile.Result{}, err
		}
	}

	pxcCR := &pxcv1.PerconaXtraDBClusterBackup{}
	err = r.Get(ctx, req.NamespacedName, pxcCR)
	if err != nil {
		return reconcile.Result{}, err
	}

	result, err := controllerutil.CreateOrUpdate(ctx, r.Client, backup, func() error {
		backup.Spec.DBClusterName = pxcCR.Spec.PXCCluster
		backup.Spec.ObjectStorageName = pxcCR.Spec.StorageName

		backup.ObjectMeta.Labels = map[string]string{
			controllers.DatabaseClusterNameLabel: pxcCR.Spec.PXCCluster,
			controllers.BackupStorageNameLabel:   pxcCR.Spec.StorageName,
		}
		backup.Status.State = everestv1alpha1.BackupState(pxcCR.Status.State)
		backup.Status.CompletedAt = pxcCR.Status.CompletedAt
		backup.Status.CreatedAt = &pxcCR.CreationTimestamp
		backup.Status.Destination = &pxcCR.Status.Destination

		return nil
	})
	if err != nil {
		return reconcile.Result{}, err
	}

	// set the created backup CR as OwnerReference to the PXC backup
	if result == controllerutil.OperationResultCreated {
		refs := append(pxcCR.OwnerReferences, metav1.OwnerReference{
			APIVersion: controllers.DatabaseClusterBackupAPI,
			Kind:       controllers.DatabaseClusterBackupKind,
			Name:       backup.Name,
			UID:        backup.UID,
		})
		pxcCR.OwnerReferences = refs
		err := r.Update(ctx, pxcCR)
		if err != nil {
			logger.Error(err, "Failed to set ownership for a PXC backup")
		}
	}

	logger.Info("Reconciled", "request", req)
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DatabaseClusterPXCBackupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&pxcv1.PerconaXtraDBClusterBackup{}).Complete(r)
}
