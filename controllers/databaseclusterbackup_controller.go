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

package controllers

import (
	"context"

	pgv2beta1 "github.com/percona/percona-postgresql-operator/pkg/apis/pg.percona.com/v2beta1"
	psmdbv1 "github.com/percona/percona-server-mongodb-operator/pkg/apis/psmdb/v1"
	pxcv1 "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
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
)

const (
	pxcBackupKind    = "PerconaXtraDBClusterBackup"
	pxcBackupAPI     = "pxc.percona.com/v1"
	pxcBackupCRDName = "perconaxtradbclusterbackups.pxc.percona.com"

	psmdbBackupKind    = "PerconaServerMongoDBBackup"
	psmdbBackupAPI     = "psmdb.percona.com/v1"
	psmdbBackupCRDName = "perconaservermongodbbackups.psmdb.percona.com"

	pgBackupKind    = "PerconaPGBackup"
	pgBackupAPI     = "pg.percona.com/v2beta1"
	pgBackupCRDName = "perconapgbackups.pg.percona.com"
)

// DatabaseClusterBackupReconciler reconciles a DatabaseClusterBackup object.
type DatabaseClusterBackupReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=everest.percona.com,resources=databaseclusterbackups,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=everest.percona.com,resources=databaseclusterbackups/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=everest.percona.com,resources=databaseclusterbackups/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// Modify the Reconcile function to compare the state specified by
// the DatabaseClusterBackup object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.1/pkg/reconcile
func (r *DatabaseClusterBackupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling", "request", req)

	backup := &everestv1alpha1.DatabaseClusterBackup{}

	err := r.Get(ctx, req.NamespacedName, backup)
	if err != nil {
		if err = client.IgnoreNotFound(err); err != nil {
			logger.Error(err, "unable to fetch DatabaseCluster")
		}
		return reconcile.Result{}, err
	}

	if backup.Spec.DBClusterType == "pxc" {
		if err = r.reconcilePXC(ctx, backup); err != nil {
			logger.Error(err, "failed to reconcile PXC backup")
			return reconcile.Result{}, err
		}
	}

	if backup.Spec.DBClusterType == "psmdb" {
		if err = r.reconcilePSMDB(ctx, backup); err != nil {
			logger.Error(err, "failed to reconcile PXC backup")
			return reconcile.Result{}, err
		}
	}

	if backup.Spec.DBClusterType == "pg" {
		if err = r.reconcilePG(ctx, backup); err != nil {
			logger.Error(err, "failed to reconcile PXC backup")
			return reconcile.Result{}, err
		}
	}

	logger.Info("Reconciled", "request", req)
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DatabaseClusterBackupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	unstructuredResource := &unstructured.Unstructured{}
	unstructuredResource.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "apiextensions.k8s.io",
		Kind:    "CustomResourceDefinition",
		Version: "v1",
	})
	controller := ctrl.NewControllerManagedBy(mgr).
		For(&everestv1alpha1.DatabaseClusterBackup{})

	ctx := context.Background()

	err := r.Get(ctx, types.NamespacedName{Name: pxcBackupCRDName}, unstructuredResource)
	if err == nil {
		if err := r.addPXCToScheme(r.Scheme); err == nil {
			controller.Owns(&pxcv1.PerconaXtraDBClusterBackup{})
		}
	}

	err = r.Get(ctx, types.NamespacedName{Name: psmdbBackupCRDName}, unstructuredResource)
	if err == nil {
		if err := r.addPSMDBToScheme(r.Scheme); err == nil {
			controller.Owns(&psmdbv1.PerconaServerMongoDBBackup{})
		}
	}

	err = r.Get(ctx, types.NamespacedName{Name: pgBackupCRDName}, unstructuredResource)
	if err == nil {
		if err := r.addPGToScheme(r.Scheme); err == nil {
			controller.Owns(&pgv2beta1.PerconaPGBackup{})
		}
	}
	return controller.Complete(r)
}

func (r *DatabaseClusterBackupReconciler) addPXCToScheme(scheme *runtime.Scheme) error {
	builder := runtime.NewSchemeBuilder(r.addPXCKnownTypes)
	return builder.AddToScheme(scheme)
}

func (r *DatabaseClusterBackupReconciler) addPXCKnownTypes(scheme *runtime.Scheme) error {
	pxcSchemeGroupVersion := schema.GroupVersion{Group: "pxc.percona.com", Version: "v1"}
	scheme.AddKnownTypes(pxcSchemeGroupVersion,
		&pxcv1.PerconaXtraDBClusterBackup{}, &pxcv1.PerconaXtraDBClusterBackupList{})

	metav1.AddToGroupVersion(scheme, pxcSchemeGroupVersion)
	return nil
}

func (r *DatabaseClusterBackupReconciler) addPSMDBToScheme(scheme *runtime.Scheme) error {
	builder := runtime.NewSchemeBuilder(r.addPSMDBKnownTypes)
	return builder.AddToScheme(scheme)
}

func (r *DatabaseClusterBackupReconciler) addPSMDBKnownTypes(scheme *runtime.Scheme) error {
	psmdbSchemeGroupVersion := schema.GroupVersion{Group: "psmdb.percona.com", Version: "v1"}
	scheme.AddKnownTypes(psmdbSchemeGroupVersion,
		&psmdbv1.PerconaServerMongoDBBackup{}, &psmdbv1.PerconaServerMongoDBBackupList{})

	metav1.AddToGroupVersion(scheme, psmdbSchemeGroupVersion)
	return nil
}

func (r *DatabaseClusterBackupReconciler) addPGToScheme(scheme *runtime.Scheme) error {
	builder := runtime.NewSchemeBuilder(r.addPGKnownTypes)
	return builder.AddToScheme(scheme)
}

func (r *DatabaseClusterBackupReconciler) addPGKnownTypes(scheme *runtime.Scheme) error {
	pgSchemeGroupVersion := schema.GroupVersion{Group: "pg.percona.com", Version: "v2beta1"}
	scheme.AddKnownTypes(pgSchemeGroupVersion,
		&pgv2beta1.PerconaPGBackup{}, &pgv2beta1.PerconaPGBackupList{})

	metav1.AddToGroupVersion(scheme, pgSchemeGroupVersion)
	return nil
}

func (r *DatabaseClusterBackupReconciler) reconcilePXC(ctx context.Context, backup *everestv1alpha1.DatabaseClusterBackup) error { //nolint:dupl
	pxcCR := &pxcv1.PerconaXtraDBClusterBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      backup.Name,
			Namespace: backup.Namespace,
		},
	}
	if err := controllerutil.SetControllerReference(backup, pxcCR, r.Client.Scheme()); err != nil {
		return err
	}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, pxcCR, func() error {
		pxcCR.TypeMeta = metav1.TypeMeta{
			APIVersion: pxcBackupAPI,
			Kind:       pxcBackupKind,
		}
		pxcCR.Spec.PXCCluster = backup.Spec.DBClusterName
		pxcCR.Spec.StorageName = backup.Spec.DBClusterName

		return nil
	})
	if err != nil {
		return err
	}
	pxcCR = &pxcv1.PerconaXtraDBClusterBackup{}
	err = r.Get(ctx, types.NamespacedName{Name: backup.Name, Namespace: backup.Namespace}, pxcCR)
	if err != nil {
		return err
	}
	backup.Status.State = everestv1alpha1.BackupState(pxcCR.Status.State)
	backup.Status.Completed = pxcCR.Status.CompletedAt
	return r.Status().Update(ctx, backup)
}

func (r *DatabaseClusterBackupReconciler) reconcilePSMDB(ctx context.Context, backup *everestv1alpha1.DatabaseClusterBackup) error { //nolint:dupl
	psmdbCR := &psmdbv1.PerconaServerMongoDBBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      backup.Name,
			Namespace: backup.Namespace,
		},
	}
	if err := controllerutil.SetControllerReference(backup, psmdbCR, r.Client.Scheme()); err != nil {
		return err
	}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, psmdbCR, func() error {
		psmdbCR.TypeMeta = metav1.TypeMeta{
			APIVersion: psmdbBackupAPI,
			Kind:       psmdbBackupKind,
		}
		psmdbCR.Spec.PSMDBCluster = backup.Spec.DBClusterName
		psmdbCR.Spec.StorageName = backup.Spec.DBClusterName

		return nil
	})
	if err != nil {
		return err
	}
	psmdbCR = &psmdbv1.PerconaServerMongoDBBackup{}
	err = r.Get(ctx, types.NamespacedName{Name: backup.Name, Namespace: backup.Namespace}, psmdbCR)
	if err != nil {
		return err
	}
	backup.Status.State = everestv1alpha1.BackupState(psmdbCR.Status.State)
	backup.Status.Completed = psmdbCR.Status.CompletedAt
	return r.Status().Update(ctx, backup)
}

func (r *DatabaseClusterBackupReconciler) reconcilePG(ctx context.Context, backup *everestv1alpha1.DatabaseClusterBackup) error { //nolint:dupl //may change in the future
	pgCR := &pgv2beta1.PerconaPGBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      backup.Name,
			Namespace: backup.Namespace,
		},
	}
	if err := controllerutil.SetControllerReference(backup, pgCR, r.Client.Scheme()); err != nil {
		return err
	}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, pgCR, func() error {
		pgCR.TypeMeta = metav1.TypeMeta{
			APIVersion: pgBackupAPI,
			Kind:       pgBackupKind,
		}
		pgCR.Spec.PGCluster = backup.Spec.DBClusterName
		pgCR.Spec.RepoName = backup.Spec.DBClusterName

		return nil
	})
	if err != nil {
		return err
	}
	pgCR = &pgv2beta1.PerconaPGBackup{}
	err = r.Get(ctx, types.NamespacedName{Name: backup.Name, Namespace: backup.Namespace}, pgCR)
	if err != nil {
		return err
	}
	backup.Status.State = everestv1alpha1.BackupState(pgCR.Status.State)
	backup.Status.Completed = pgCR.Status.CompletedAt
	return r.Status().Update(ctx, backup)
}
