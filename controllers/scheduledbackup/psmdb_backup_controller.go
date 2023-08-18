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

// Package scheduledbackup contains controllers for scheduled backups
package scheduledbackup //nolint:dupl

import (
	"context"
	"fmt"

	psmdbv1 "github.com/percona/percona-server-mongodb-operator/pkg/apis/psmdb/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	everestv1alpha1 "github.com/percona/everest-operator/api/v1alpha1"
	"github.com/percona/everest-operator/controllers"
)

// DatabaseClusterPSMDBBackupReconciler reconciles a DatabaseClusterBackup object.
type DatabaseClusterPSMDBBackupReconciler struct {
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
func (r *DatabaseClusterPSMDBBackupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling", "request", req)

	psmdbBackup := &psmdbv1.PerconaServerMongoDBBackup{}
	err := r.Get(ctx, req.NamespacedName, psmdbBackup)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}

	backup := &everestv1alpha1.DatabaseClusterBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Name,
			Namespace: req.Namespace,
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       controllers.DatabaseClusterBackupKind,
			APIVersion: controllers.EverestAPIVersion,
		},
	}

	err = r.Get(ctx, req.NamespacedName, backup)
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			return reconcile.Result{}, err
		}

		// if there is no such DatabaseBackup, but the OwnerReference on the psmdb backup is already set - do nothing
		// to prevent recreating DatabaseBackup after it's been deleted.
		if len(psmdbBackup.OwnerReferences) > 0 {
			return ctrl.Result{}, nil
		}
	}

	result, err := controllerutil.CreateOrUpdate(ctx, r.Client, backup, func() error {
		backup.Spec.DBClusterName = psmdbBackup.Spec.ClusterName
		backup.Spec.BackupStorageName = psmdbBackup.Spec.StorageName

		backup.ObjectMeta.Labels = map[string]string{
			controllers.DatabaseClusterNameLabel:                                              psmdbBackup.Spec.ClusterName,
			fmt.Sprintf(controllers.BackupStorageNameLabelTmpl, psmdbBackup.Spec.StorageName): controllers.BackupStorageLabelValue,
		}
		return nil
	})
	if err != nil {
		return reconcile.Result{}, err
	}

	// set the created backup CR as OwnerReference to the PSMDB backup
	if result == controllerutil.OperationResultCreated {
		psmdbBackup.OwnerReferences = []metav1.OwnerReference{{
			APIVersion: controllers.EverestAPIVersion,
			Kind:       controllers.DatabaseClusterBackupKind,
			Name:       backup.Name,
			UID:        backup.UID,
		}}
		err := r.Update(ctx, psmdbBackup)
		if err != nil {
			logger.Error(err, "Failed to set ownership for a PSMDB backup")
		}
	}

	err = r.updateStatus(ctx, req.NamespacedName, psmdbBackup)
	if err != nil {
		return reconcile.Result{}, err
	}

	logger.Info("Reconciled", "request", req)
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DatabaseClusterPSMDBBackupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	unstructuredResource := &unstructured.Unstructured{}
	unstructuredResource.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "apiextensions.k8s.io",
		Kind:    "CustomResourceDefinition",
		Version: "v1",
	})

	ctx := context.Background()

	// the manager got set up only if the upstream backup CRD is available
	err := r.Get(ctx, types.NamespacedName{Name: controllers.PSMDBBackupCRDName}, unstructuredResource)
	if err == nil {
		return ctrl.NewControllerManagedBy(mgr).
			For(&psmdbv1.PerconaServerMongoDBBackup{}).Complete(r)
	}

	logger := log.FromContext(ctx)
	logger.Info(fmt.Sprintf("DatabaseClusterPSMDBBackupReconciler is not set, no %s CRD found", controllers.PSMDBBackupCRDName))

	return nil
}

func (r *DatabaseClusterPSMDBBackupReconciler) updateStatus(
	ctx context.Context,
	namespacedName types.NamespacedName,
	psmdbBackup *psmdbv1.PerconaServerMongoDBBackup,
) error {
	backup := &everestv1alpha1.DatabaseClusterBackup{}
	err := r.Get(ctx, namespacedName, backup)
	if err != nil {
		return err
	}

	backup.Status.State = everestv1alpha1.BackupState(psmdbBackup.Status.State)
	backup.Status.CompletedAt = psmdbBackup.Status.CompletedAt
	backup.Status.CreatedAt = &psmdbBackup.CreationTimestamp
	backup.Status.Destination = &psmdbBackup.Status.Destination

	return r.Status().Update(ctx, backup)
}
