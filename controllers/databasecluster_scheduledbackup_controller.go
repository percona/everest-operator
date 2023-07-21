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
	"strconv"
	"strings"

	pgv2 "github.com/percona/percona-postgresql-operator/pkg/apis/pgv2.percona.com/v2"
	psmdbv1 "github.com/percona/percona-server-mongodb-operator/pkg/apis/psmdb/v1"
	pxcv1 "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/pkg/errors"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	everestv1alpha1 "github.com/percona/everest-operator/api/v1alpha1"
)

const (
	backupKind    = "DatabaseClusterBackup"
	backupAPI     = "everest.percona.com/everestv1alpha1"
	backupCRDName = "databaseclusterbackups.everest.percona.com"

	// PerconaXtraDBClusterBackupKind represents pxc backup kind.
	PerconaXtraDBClusterBackupKind = "PerconaXtraDBClusterBackup"
	// PerconaServerMongoDBBackupKind represents psmdb backup kind.
	PerconaServerMongoDBBackupKind = "PerconaServerMongoDBBackup"
	// PerconaPGClusterBackupKind represents postgresql backup kind.
	PerconaPGClusterBackupKind = "PerconaPGClusterBackup"
)

// DatabaseClusterScheduledBackupReconciler reconciles a sheduled DatabaseClusterBackup object.
type DatabaseClusterScheduledBackupReconciler struct {
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
func (r *DatabaseClusterScheduledBackupReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling", "request", req)

	job := &batchv1.Job{}

	err := r.Get(ctx, req.NamespacedName, job)
	if err != nil {
		if err = client.IgnoreNotFound(err); err != nil {
			logger.Error(err, "unable to fetch job")
		}
		return reconcile.Result{}, err
	}

	for _, ref := range job.OwnerReferences {
		err = r.reconcileUpstream(ctx, ref, job)
		if err != nil {
			logger.Error(err, "failed to reconcile database backup")
			return ctrl.Result{}, err
		}
	}

	logger.Info("Reconciled", "request", req)
	return ctrl.Result{}, nil
}

func (r *DatabaseClusterScheduledBackupReconciler) reconcileUpstream(ctx context.Context, ref metav1.OwnerReference, job *batchv1.Job) error {
	namespacedName := types.NamespacedName{
		Namespace: job.Namespace,
		Name:      ref.Name,
	}
	switch ref.Kind {
	case PerconaXtraDBClusterBackupKind:
		backup := &pxcv1.PerconaXtraDBClusterBackup{}
		err := r.Get(ctx, namespacedName, backup)
		if err == nil {
			if err = r.reconcilePXC(ctx, backup); err != nil {
				return err
			}
		}
	case PerconaServerMongoDBBackupKind:
		backup := &psmdbv1.PerconaServerMongoDBBackup{}
		err := r.Get(ctx, namespacedName, backup)
		if err == nil {
			if err = r.reconcilePSMDB(ctx, backup); err != nil {
				return err
			}
		}

	case PerconaPGClusterBackupKind:
		backup := &pgv2.PerconaPGBackup{}
		err := r.Get(ctx, namespacedName, backup)
		if err == nil {
			if err = r.reconcilePG(ctx, backup); err != nil {
				return err
			}
		}
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DatabaseClusterScheduledBackupReconciler) SetupWithManager(mgr ctrl.Manager) error {
	unstructuredResource := &unstructured.Unstructured{}
	unstructuredResource.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "apiextensions.k8s.io",
		Kind:    "CustomResourceDefinition",
		Version: "v1",
	})

	ctx := context.Background()

	err := r.Get(ctx, types.NamespacedName{Name: pxcBackupCRDName}, unstructuredResource)
	if err == nil {
		if err := r.addPXCToScheme(r.Scheme); err != nil {
			return err
		}
	}

	err = r.Get(ctx, types.NamespacedName{Name: psmdbBackupCRDName}, unstructuredResource)
	if err == nil {
		if err := r.addPSMDBToScheme(r.Scheme); err != nil {
			return err
		}
	}

	err = r.Get(ctx, types.NamespacedName{Name: pgBackupCRDName}, unstructuredResource)
	if err == nil {
		if err := r.addPGToScheme(r.Scheme); err != nil {
			return err
		}
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&batchv1.Job{}).
		WithEventFilter(scheduledBackupPredicate()).
		Complete(r)
}

func scheduledBackupPredicate() predicate.Predicate { //nolint:ireturn
	return predicate.Funcs{
		UpdateFunc: func(e event.UpdateEvent) bool {
			return checkKind(e.ObjectNew.GetOwnerReferences())
		},
		CreateFunc: func(e event.CreateEvent) bool {
			return checkKind(e.Object.GetOwnerReferences())
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return false
		},
	}
}

func checkKind(refs []metav1.OwnerReference) bool {
	for _, ref := range refs {
		return ref.Kind == PerconaXtraDBClusterBackupKind || ref.Kind == PerconaServerMongoDBBackupKind || ref.Kind == PerconaPGClusterBackupKind
	}
	return false
}

func (r *DatabaseClusterScheduledBackupReconciler) addPXCToScheme(scheme *runtime.Scheme) error {
	builder := runtime.NewSchemeBuilder(r.addPXCKnownTypes)
	return builder.AddToScheme(scheme)
}

func (r *DatabaseClusterScheduledBackupReconciler) addPXCKnownTypes(scheme *runtime.Scheme) error {
	pxcSchemeGroupVersion := schema.GroupVersion{Group: "pxc.percona.com", Version: "v1"}
	scheme.AddKnownTypes(pxcSchemeGroupVersion,
		&pxcv1.PerconaXtraDBClusterBackup{}, &pxcv1.PerconaXtraDBClusterBackupList{})

	metav1.AddToGroupVersion(scheme, pxcSchemeGroupVersion)
	return nil
}

func (r *DatabaseClusterScheduledBackupReconciler) addPSMDBToScheme(scheme *runtime.Scheme) error {
	builder := runtime.NewSchemeBuilder(r.addPSMDBKnownTypes)
	return builder.AddToScheme(scheme)
}

func (r *DatabaseClusterScheduledBackupReconciler) addPSMDBKnownTypes(scheme *runtime.Scheme) error {
	psmdbSchemeGroupVersion := schema.GroupVersion{Group: "psmdb.percona.com", Version: "v1"}
	scheme.AddKnownTypes(psmdbSchemeGroupVersion,
		&psmdbv1.PerconaServerMongoDBBackup{}, &psmdbv1.PerconaServerMongoDBBackupList{})

	metav1.AddToGroupVersion(scheme, psmdbSchemeGroupVersion)
	return nil
}

func (r *DatabaseClusterScheduledBackupReconciler) addPGToScheme(scheme *runtime.Scheme) error {
	builder := runtime.NewSchemeBuilder(r.addPGKnownTypes)
	return builder.AddToScheme(scheme)
}

func (r *DatabaseClusterScheduledBackupReconciler) addPGKnownTypes(scheme *runtime.Scheme) error {
	pgSchemeGroupVersion := schema.GroupVersion{Group: "pgv2.percona.com", Version: "v2"}
	scheme.AddKnownTypes(pgSchemeGroupVersion,
		&pgv2.PerconaPGBackup{}, &pgv2.PerconaPGBackupList{})

	metav1.AddToGroupVersion(scheme, pgSchemeGroupVersion)
	return nil
}

func (r *DatabaseClusterScheduledBackupReconciler) reconcilePXC(ctx context.Context, pxcBackup *pxcv1.PerconaXtraDBClusterBackup) error { //nolint:dupl
	backup := &everestv1alpha1.DatabaseClusterBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pxcBackup.Name,
			Namespace: pxcBackup.Namespace,
		},
	}
	if err := controllerutil.SetControllerReference(pxcBackup, backup, r.Client.Scheme()); err != nil {
		return err
	}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, backup, func() error {
		backup.TypeMeta = metav1.TypeMeta{
			APIVersion: backupAPI,
			Kind:       backupKind,
		}
		backup.Spec.DBClusterName = pxcBackup.Spec.PXCCluster
		backup.Spec.BackupSource.StorageName = pxcBackup.Spec.StorageName
		backup.Spec.EngineType = string(everestv1alpha1.DatabaseEnginePXC)
		return nil
	})
	if err != nil {
		return err
	}
	backup = &everestv1alpha1.DatabaseClusterBackup{}
	err = r.Get(ctx, types.NamespacedName{Name: pxcBackup.Name, Namespace: pxcBackup.Namespace}, backup)
	if err != nil {
		return err
	}
	backup.Status.State = everestv1alpha1.BackupState(pxcBackup.Status.State)
	backup.Status.CreatedAt = &pxcBackup.CreationTimestamp
	backup.Status.CompletedAt = pxcBackup.Status.CompletedAt
	backup.Status.Destination = &pxcBackup.Status.Destination
	return r.Status().Update(ctx, backup)
}

func (r *DatabaseClusterScheduledBackupReconciler) reconcilePSMDB(ctx context.Context, psmdbBackup *psmdbv1.PerconaServerMongoDBBackup) error { //nolint:dupl
	backup := &everestv1alpha1.DatabaseClusterBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      psmdbBackup.Name,
			Namespace: psmdbBackup.Namespace,
		},
	}
	if err := controllerutil.SetControllerReference(psmdbBackup, backup, r.Client.Scheme()); err != nil {
		return err
	}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, backup, func() error {
		backup.TypeMeta = metav1.TypeMeta{
			APIVersion: backupAPI,
			Kind:       backupKind,
		}
		backup.Spec.DBClusterName = psmdbBackup.Spec.PSMDBCluster
		backup.Spec.BackupSource.StorageName = psmdbBackup.Spec.StorageName
		backup.Spec.EngineType = string(everestv1alpha1.DatabaseEnginePSMDB)
		return nil
	})
	if err != nil {
		return err
	}
	backup = &everestv1alpha1.DatabaseClusterBackup{}
	err = r.Get(ctx, types.NamespacedName{Name: psmdbBackup.Name, Namespace: psmdbBackup.Namespace}, backup)
	if err != nil {
		return err
	}
	backup.Status.State = everestv1alpha1.BackupState(psmdbBackup.Status.State)
	backup.Status.CreatedAt = &psmdbBackup.CreationTimestamp
	backup.Status.CompletedAt = psmdbBackup.Status.CompletedAt
	backup.Status.Destination = &psmdbBackup.Status.Destination
	return r.Status().Update(ctx, backup)
}

func (r *DatabaseClusterScheduledBackupReconciler) reconcilePG(ctx context.Context, pgBackup *pgv2.PerconaPGBackup) error {
	backup := &everestv1alpha1.DatabaseClusterBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pgBackup.Name,
			Namespace: pgBackup.Namespace,
		},
	}
	if err := controllerutil.SetControllerReference(pgBackup, backup, r.Client.Scheme()); err != nil {
		return err
	}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, backup, func() error {
		backup.TypeMeta = metav1.TypeMeta{
			APIVersion: backupAPI,
			Kind:       backupKind,
		}
		backup.Spec.DBClusterName = pgBackup.Spec.PGCluster

		storageName, err := r.pgStorageName(ctx, pgBackup)
		if err != nil {
			return err
		}
		backup.Spec.BackupSource.StorageName = storageName
		backup.Spec.EngineType = string(everestv1alpha1.DatabaseEnginePostgresql)
		return nil
	})
	if err != nil {
		return err
	}
	backup = &everestv1alpha1.DatabaseClusterBackup{}
	err = r.Get(ctx, types.NamespacedName{Name: pgBackup.Name, Namespace: pgBackup.Namespace}, backup)
	if err != nil {
		return err
	}
	backup.Status.State = everestv1alpha1.BackupState(pgBackup.Status.State)
	backup.Status.CreatedAt = &pgBackup.CreationTimestamp
	backup.Status.CompletedAt = pgBackup.Status.CompletedAt
	return r.Status().Update(ctx, backup)
}

// storageName returns ObjectStorageName that corresponds the PG cluster repo name
// which is like "repo1", "repo2" etc.
func (r *DatabaseClusterScheduledBackupReconciler) pgStorageName(ctx context.Context, pgBackup *pgv2.PerconaPGBackup) (string, error) {
	idx, err := strconv.Atoi(strings.TrimPrefix(pgBackup.Spec.RepoName, "repo"))
	if err != nil {
		return "", err
	}

	cluster := &everestv1alpha1.DatabaseCluster{}
	err = r.Get(ctx, types.NamespacedName{Name: pgBackup.Spec.PGCluster, Namespace: pgBackup.Namespace}, cluster)
	if err != nil {
		return "", err
	}

	if len(cluster.Spec.Backup.Schedules) < idx {
		return "", errors.Errorf("repo with index %v is not defined", idx)
	}

	return cluster.Spec.Backup.Schedules[idx-1].ObjectStorageName, nil
}
