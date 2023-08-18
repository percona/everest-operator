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
	"fmt"

	pgv2 "github.com/percona/percona-postgresql-operator/pkg/apis/pgv2.percona.com/v2"
	psmdbv1 "github.com/percona/percona-server-mongodb-operator/pkg/apis/psmdb/v1"
	pxcv1 "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/pkg/errors"
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
	// DatabaseClusterBackupKind the kind for the DatabaseClusterBackup.
	DatabaseClusterBackupKind = "DatabaseClusterBackup"
	// EverestAPIVersion the APIVersion for the everest resources.
	EverestAPIVersion = "everest.percona.com/v1alpha1"

	// PXCBackupKind the kind for the PXCBackup.
	PXCBackupKind = "PerconaXtraDBClusterBackup"
	// PXCAPIVersion the APIVersion for the pxc operator resources.
	PXCAPIVersion = "pxc.percona.com/v1"
	// PXCBackupCRDName the name of the pxc CRD for the pxc-backups.
	PXCBackupCRDName = "perconaxtradbclusterbackups.pxc.percona.com"

	psmdbBackupKind = "PerconaServerMongoDBBackup"
	psmdbAPIVersion = "psmdb.percona.com/v1"
	// PSMDBBackupCRDName the name of the psmdb CRD for the psmdb-backups.
	PSMDBBackupCRDName = "perconaservermongodbbackups.psmdb.percona.com"

	pgBackupKind = "PerconaPGBackup"
	pgAPIVersion = "pgv2.percona.com/v2"
	// PGBackupCRDName the name of the pg CRD for the pg-backups.
	PGBackupCRDName = "perconapgbackups.pgv2.percona.com"
)

// DatabaseClusterBackupReconciler reconciles a DatabaseClusterBackup object.
type DatabaseClusterBackupReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=everest.percona.com,resources=databaseclusterbackups,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=everest.percona.com,resources=databaseclusterbackups/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=everest.percona.com,resources=databaseclusterbackups/finalizers,verbs=update
// +kubebuilder:rbac:groups=pxc.percona.com,resources=perconaxtradbclusterbackups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=psmdb.percona.com,resources=perconaservermongodbbackups,verbs=get;list;watch;create;update;patch;delete
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
func (r *DatabaseClusterBackupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling", "request", req)

	backup := &everestv1alpha1.DatabaseClusterBackup{}

	err := r.Get(ctx, req.NamespacedName, backup)
	if err != nil {
		if err = client.IgnoreNotFound(err); err != nil {
			logger.Error(err, "unable to fetch DatabaseClusterBackup")
		}
		return reconcile.Result{}, err
	}
	if len(backup.ObjectMeta.Labels) == 0 {
		backup.ObjectMeta.Labels = map[string]string{
			DatabaseClusterNameLabel: backup.Spec.DBClusterName,
			fmt.Sprintf(BackupStorageNameLabelTmpl, backup.Spec.BackupStorageName): BackupStorageLabelValue,
		}
		if err := r.Update(ctx, backup); err != nil {
			return reconcile.Result{}, err
		}
	}

	cluster := &everestv1alpha1.DatabaseCluster{}
	err = r.Get(ctx, types.NamespacedName{Name: backup.Spec.DBClusterName, Namespace: backup.Namespace}, cluster)
	if err != nil {
		if err = client.IgnoreNotFound(err); err != nil {
			logger.Error(err, "unable to fetch DatabaseCluster")
		}
		return reconcile.Result{}, err
	}

	storage := &everestv1alpha1.BackupStorage{}
	err = r.Get(ctx, types.NamespacedName{Name: backup.Spec.BackupStorageName, Namespace: backup.Namespace}, storage)
	if err != nil {
		if err = client.IgnoreNotFound(err); err != nil {
			logger.Error(err, "unable to fetch BackupStorage")
		}
		return reconcile.Result{}, err
	}

	switch cluster.Spec.Engine.Type {
	case everestv1alpha1.DatabaseEnginePXC:
		if err = r.reconcilePXC(ctx, backup); err != nil {
			logger.Error(err, "failed to reconcile PXC backup")
			return reconcile.Result{}, err
		}
	case everestv1alpha1.DatabaseEnginePSMDB:
		if err = r.reconcilePSMDB(ctx, backup); err != nil {
			logger.Error(err, "failed to reconcile PSMDB backup")
			return reconcile.Result{}, err
		}
	case everestv1alpha1.DatabaseEnginePostgresql:
		if err = r.reconcilePG(ctx, backup, cluster); err != nil {
			logger.Error(err, "failed to reconcile PG backup")
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

	err := r.Get(ctx, types.NamespacedName{Name: PXCBackupCRDName}, unstructuredResource)
	if err == nil {
		if err := r.addPXCToScheme(r.Scheme); err == nil {
			controller.Owns(&pxcv1.PerconaXtraDBClusterBackup{})
		}
	}

	err = r.Get(ctx, types.NamespacedName{Name: PSMDBBackupCRDName}, unstructuredResource)
	if err == nil {
		if err := r.addPSMDBToScheme(r.Scheme); err == nil {
			controller.Owns(&psmdbv1.PerconaServerMongoDBBackup{})
		}
	}

	err = r.Get(ctx, types.NamespacedName{Name: PGBackupCRDName}, unstructuredResource)
	if err == nil {
		if err := r.addPGToScheme(r.Scheme); err == nil {
			controller.Owns(&pgv2.PerconaPGBackup{})
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
	pgSchemeGroupVersion := schema.GroupVersion{Group: "pgv2.percona.com", Version: "v2"}
	scheme.AddKnownTypes(pgSchemeGroupVersion,
		&pgv2.PerconaPGBackup{}, &pgv2.PerconaPGBackupList{})

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
			APIVersion: PXCAPIVersion,
			Kind:       PXCBackupKind,
		}
		pxcCR.Spec.PXCCluster = backup.Spec.DBClusterName
		pxcCR.Spec.StorageName = backup.Spec.BackupStorageName

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
	backup.Status.CompletedAt = pxcCR.Status.CompletedAt
	backup.Status.CreatedAt = &pxcCR.CreationTimestamp
	backup.Status.Destination = &pxcCR.Status.Destination

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
			APIVersion: psmdbAPIVersion,
			Kind:       psmdbBackupKind,
		}
		psmdbCR.Spec.PSMDBCluster = backup.Spec.DBClusterName
		psmdbCR.Spec.StorageName = backup.Spec.BackupStorageName

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
	backup.Status.CompletedAt = psmdbCR.Status.CompletedAt
	backup.Status.CreatedAt = &psmdbCR.CreationTimestamp
	backup.Status.Destination = &psmdbCR.Status.Destination
	return r.Status().Update(ctx, backup)
}

func (r *DatabaseClusterBackupReconciler) reconcilePG(
	ctx context.Context,
	backup *everestv1alpha1.DatabaseClusterBackup,
	dbcluster *everestv1alpha1.DatabaseCluster,
) error {
	pgCR := &pgv2.PerconaPGBackup{
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
			APIVersion: pgAPIVersion,
			Kind:       pgBackupKind,
		}
		pgCR.Spec.PGCluster = backup.Spec.DBClusterName

		repoName, err := r.pgRepoName(backup, dbcluster)
		if err != nil {
			return err
		}
		pgCR.Spec.RepoName = repoName
		return nil
	})
	if err != nil {
		return err
	}
	pgCR = &pgv2.PerconaPGBackup{}
	err = r.Get(ctx, types.NamespacedName{Name: backup.Name, Namespace: backup.Namespace}, pgCR)
	if err != nil {
		return err
	}
	backup.Status.State = everestv1alpha1.BackupState(pgCR.Status.State)
	backup.Status.CompletedAt = pgCR.Status.CompletedAt
	backup.Status.CreatedAt = &pgCR.CreationTimestamp
	//nolint:godox
	// TODO: add backup.Status.Destination once https://jira.percona.com/browse/K8SPG-411 is done
	return r.Status().Update(ctx, backup)
}

// pgRepoName returns the pg cluster's RepoName (which is like "repo1", "repo2" etc)
// that corresponds the database cluster's BackupStorageName.
func (r *DatabaseClusterBackupReconciler) pgRepoName(everestBackup *everestv1alpha1.DatabaseClusterBackup, dbcluster *everestv1alpha1.DatabaseCluster) (string, error) {
	//nolint:godox
	// TODO: decouple PG backups from the schedules list
	// currently the PG on-demand backups require at least one schedule to be set,
	// here is an idea how to fix it https://github.com/percona/everest-operator/pull/7#discussion_r1263497633
	for idx, schedule := range dbcluster.Spec.Backup.Schedules {
		if schedule.BackupStorageName == everestBackup.Spec.BackupStorageName {
			return fmt.Sprintf("repo%d", idx+1), nil
		}
	}

	return "", errors.Errorf("can't perform the backup because the requested backup location isn't set in the DatabaseCluster")
}
