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
	"crypto/tls"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/AlekSi/pointer"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	pgv2 "github.com/percona/percona-postgresql-operator/pkg/apis/pgv2.percona.com/v2"
	psmdbv1 "github.com/percona/percona-server-mongodb-operator/pkg/apis/psmdb/v1"
	pxcv1 "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	everestv1alpha1 "github.com/percona/everest-operator/api/v1alpha1"
	"github.com/percona/everest-operator/controllers/common"
)

const (
	databaseClusterBackupKind = "DatabaseClusterBackup"
	everestAPIVersion         = "everest.percona.com/v1alpha1"

	pxcBackupKind    = "PerconaXtraDBClusterBackup"
	pxcAPIVersion    = "pxc.percona.com/v1"
	pxcBackupCRDName = "perconaxtradbclusterbackups.pxc.percona.com"

	psmdbBackupKind    = "PerconaServerMongoDBBackup"
	psmdbAPIVersion    = "psmdb.percona.com/v1"
	psmdbBackupCRDName = "perconaservermongodbbackups.psmdb.percona.com"

	pgBackupKind    = "PerconaPGBackup"
	pgAPIVersion    = "pgv2.percona.com/v2"
	pgBackupCRDName = "perconapgbackups.pgv2.percona.com"

	dbClusterBackupDBClusterNameField = ".spec.dbClusterName"
	pxcGapsReasonString               = "BinlogGapDetected"

	deletePXCBackupFinalizer   = "delete-s3-backup"
	deletePSMDBBackupFinalizer = "delete-backup"
)

// ErrBackupStorageUndefined is returned when a backup storage is not defined
// in the corresponding upstream DB cluster CR.
var ErrBackupStorageUndefined = errors.New("backup storage is not defined in the upstream DB cluster CR")

// DatabaseClusterBackupReconciler reconciles a DatabaseClusterBackup object.
type DatabaseClusterBackupReconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	systemNamespace string
}

//+kubebuilder:rbac:groups=everest.percona.com,resources=databaseclusterbackups,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=everest.percona.com,resources=databaseclusterbackups/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=everest.percona.com,resources=databaseclusterbackups/finalizers,verbs=update
// +kubebuilder:rbac:groups=pxc.percona.com,resources=perconaxtradbclusterbackups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=psmdb.percona.com,resources=perconaservermongodbbackups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=pgv2.percona.com,resources=perconapgbackups,verbs=get;list;watch;create;update;patch;delete

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
			databaseClusterNameLabel: backup.Spec.DBClusterName,
			fmt.Sprintf(backupStorageNameLabelTmpl, backup.Spec.BackupStorageName): backupStorageLabelValue,
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
	err = r.Get(ctx, types.NamespacedName{Name: backup.Spec.BackupStorageName, Namespace: r.systemNamespace}, storage)
	if err != nil {
		if err = client.IgnoreNotFound(err); err != nil {
			logger.Error(err, "unable to fetch BackupStorage")
		}
		return reconcile.Result{}, err
	}

	// By default, we must always verify TLS.
	if verifyTLS := storage.Spec.VerifyTLS; verifyTLS == nil {
		storage.Spec.VerifyTLS = pointer.To(true)
	}

	requeue := false
	switch cluster.Spec.Engine.Type {
	case everestv1alpha1.DatabaseEnginePXC:
		requeue, err = r.reconcilePXC(ctx, backup)
	case everestv1alpha1.DatabaseEnginePSMDB:
		requeue, err = r.reconcilePSMDB(ctx, backup)
	case everestv1alpha1.DatabaseEnginePostgresql:
		requeue, err = r.reconcilePG(ctx, backup)
	}

	// The DatabaseCluster controller is responsible for updating the
	// upstream DB cluster with the necessary storage definition. If the
	// storage is not defined in the upstream DB cluster CR, we requeue the
	// backup to give the DatabaseCluster controller a chance to update the
	// upstream DB cluster CR.
	if errors.Is(err, ErrBackupStorageUndefined) {
		logger.Info(
			fmt.Sprintf("Backup storage %s is not defined in the %s cluster %s, requeuing",
				backup.Spec.BackupStorageName,
				cluster.Spec.Engine.Type,
				backup.Spec.DBClusterName),
		)
		return reconcile.Result{Requeue: true}, nil
	}

	if err != nil {
		logger.Error(err, fmt.Sprintf("failed to reconcile %s backup", cluster.Spec.Engine.Type))
		return reconcile.Result{}, err
	}

	logger.Info("Reconciled", "request", req)
	return ctrl.Result{Requeue: requeue}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DatabaseClusterBackupReconciler) SetupWithManager(mgr ctrl.Manager, systemNamespace string) error {
	unstructuredResource := &unstructured.Unstructured{}
	unstructuredResource.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "apiextensions.k8s.io",
		Kind:    "CustomResourceDefinition",
		Version: "v1",
	})

	// Index the dbClusterName field in DatabaseClusterBackup.
	err := mgr.GetFieldIndexer().IndexField(
		context.Background(), &everestv1alpha1.DatabaseClusterBackup{}, dbClusterBackupDBClusterNameField,
		func(o client.Object) []string {
			var res []string
			dbb, ok := o.(*everestv1alpha1.DatabaseClusterBackup)
			if !ok {
				return res
			}
			res = append(res, dbb.Spec.DBClusterName)
			return res
		},
	)
	if err != nil {
		return err
	}

	controller := ctrl.NewControllerManagedBy(mgr).
		For(&everestv1alpha1.DatabaseClusterBackup{})

	ctx := context.Background()

	err = r.Get(ctx, types.NamespacedName{Name: pxcBackupCRDName}, unstructuredResource)
	if err == nil {
		if err := r.addPXCToScheme(r.Scheme); err == nil {
			controller.Watches(
				&pxcv1.PerconaXtraDBClusterBackup{},
				handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
					return r.tryCreateDBBackups(ctx, obj, r.tryCreatePXC)
				}))
		}
	}

	err = r.Get(ctx, types.NamespacedName{Name: psmdbBackupCRDName}, unstructuredResource)
	if err == nil {
		if err := r.addPSMDBToScheme(r.Scheme); err == nil {
			controller.Watches(
				&psmdbv1.PerconaServerMongoDBBackup{},
				handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
					return r.tryCreateDBBackups(ctx, obj, r.tryCreatePSMDB)
				}))
		}
	}

	err = r.Get(ctx, types.NamespacedName{Name: pgBackupCRDName}, unstructuredResource)
	if err == nil {
		if err := r.addPGToScheme(r.Scheme); err == nil {
			controller.Watches(
				&pgv2.PerconaPGBackup{},
				handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
					return r.tryCreateDBBackups(ctx, obj, r.tryCreatePG)
				}))
		}
	}

	r.systemNamespace = systemNamespace

	return controller.Complete(r)
}

func (r *DatabaseClusterBackupReconciler) tryCreateDBBackups(
	ctx context.Context,
	obj client.Object,
	createBackupFunc func(ctx context.Context, obj client.Object) error,
) []reconcile.Request {
	logger := log.FromContext(ctx)
	if len(obj.GetOwnerReferences()) == 0 {
		err := createBackupFunc(ctx, obj)
		if err != nil {
			logger.Error(err, "Failed to create DatabaseClusterBackup "+obj.GetName())
		}
	}

	// needed to reconcile the DatabaseClusterBackup
	// which has the same name and namespace as the upstream backup
	return []reconcile.Request{{
		NamespacedName: types.NamespacedName{
			Name:      obj.GetName(),
			Namespace: obj.GetNamespace(),
		},
	}}
}

func (r *DatabaseClusterBackupReconciler) tryCreatePG(ctx context.Context, obj client.Object) error {
	pgBackup := &pgv2.PerconaPGBackup{}
	namespacedName := types.NamespacedName{
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}

	if err := r.Get(ctx, namespacedName, pgBackup); err != nil {
		// if such upstream backup is not found - do nothing
		if k8serrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	// We want to ignore backups that are done to the hardcoded PVC-based repo1.
	// This repo only exists to allow users to spin up a PG cluster without specifying a backup storage.
	// Therefore, we don't want to allow users to restore from these backups so shouldn't create a DBB CR from repo1.
	if pgBackup.Spec.RepoName == "repo1" {
		return nil
	}

	backup := &everestv1alpha1.DatabaseClusterBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      namespacedName.Name,
			Namespace: namespacedName.Namespace,
		},
	}

	err := r.Get(ctx, namespacedName, backup)
	// if such everest backup already exists - do nothing
	if err == nil {
		return nil
	}
	if !k8serrors.IsNotFound(err) {
		return err
	}

	backup.Spec.DBClusterName = pgBackup.Spec.PGCluster

	cluster := &everestv1alpha1.DatabaseCluster{}
	err = r.Get(ctx, types.NamespacedName{Name: pgBackup.Spec.PGCluster, Namespace: pgBackup.Namespace}, cluster)
	if err != nil {
		return err
	}
	name, nErr := backupStorageName(pgBackup.Spec.RepoName, cluster)
	if nErr != nil {
		return nErr
	}

	backup.Spec.BackupStorageName = name
	backup.ObjectMeta.Labels = map[string]string{
		databaseClusterNameLabel:                      pgBackup.Spec.PGCluster,
		fmt.Sprintf(backupStorageNameLabelTmpl, name): backupStorageLabelValue,
	}
	backup.ObjectMeta.SetOwnerReferences([]metav1.OwnerReference{{
		APIVersion:         pgAPIVersion,
		Kind:               pgBackupKind,
		Name:               pgBackup.Name,
		UID:                pgBackup.UID,
		BlockOwnerDeletion: pointer.ToBool(true),
	}})

	return r.Create(ctx, backup)
}

func (r *DatabaseClusterBackupReconciler) tryCreatePXC(ctx context.Context, obj client.Object) error {
	pxcBackup := &pxcv1.PerconaXtraDBClusterBackup{}
	namespacedName := types.NamespacedName{
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}

	if err := r.Get(ctx, namespacedName, pxcBackup); err != nil {
		// if such upstream backup is not found - do nothing
		if k8serrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	backup := &everestv1alpha1.DatabaseClusterBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      namespacedName.Name,
			Namespace: namespacedName.Namespace,
		},
	}

	err := r.Get(ctx, namespacedName, backup)
	// if such everest backup already exists - do nothing
	if err == nil {
		return nil
	}
	if !k8serrors.IsNotFound(err) {
		return err
	}

	backup.Spec.DBClusterName = pxcBackup.Spec.PXCCluster
	backup.Spec.BackupStorageName = pxcBackup.Spec.StorageName

	backup.ObjectMeta.Labels = map[string]string{
		databaseClusterNameLabel: pxcBackup.Spec.PXCCluster,
		fmt.Sprintf(backupStorageNameLabelTmpl, pxcBackup.Spec.StorageName): backupStorageLabelValue,
	}
	backup.ObjectMeta.SetOwnerReferences([]metav1.OwnerReference{{
		APIVersion:         pxcAPIVersion,
		Kind:               pxcBackupKind,
		Name:               pxcBackup.Name,
		UID:                pxcBackup.UID,
		BlockOwnerDeletion: pointer.ToBool(true),
	}})
	return r.Create(ctx, backup)
}

func (r *DatabaseClusterBackupReconciler) tryCreatePSMDB(ctx context.Context, obj client.Object) error {
	psmdbBackup := &psmdbv1.PerconaServerMongoDBBackup{}
	namespacedName := types.NamespacedName{
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}

	if err := r.Get(ctx, namespacedName, psmdbBackup); err != nil {
		// if such upstream backup is not found - do nothing
		if k8serrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	backup := &everestv1alpha1.DatabaseClusterBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      namespacedName.Name,
			Namespace: namespacedName.Namespace,
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       databaseClusterBackupKind,
			APIVersion: everestAPIVersion,
		},
	}

	err := r.Get(ctx, namespacedName, backup)
	// if such everest backup already exists - do nothing
	if err == nil {
		return nil
	}
	if !k8serrors.IsNotFound(err) {
		return err
	}

	backup.Spec.DBClusterName = psmdbBackup.Spec.ClusterName
	backup.Spec.BackupStorageName = psmdbBackup.Spec.StorageName

	backup.ObjectMeta.Labels = map[string]string{
		databaseClusterNameLabel: psmdbBackup.Spec.ClusterName,
		fmt.Sprintf(backupStorageNameLabelTmpl, psmdbBackup.Spec.StorageName): backupStorageLabelValue,
	}
	backup.ObjectMeta.SetOwnerReferences([]metav1.OwnerReference{{
		APIVersion:         psmdbAPIVersion,
		Kind:               psmdbBackupKind,
		Name:               psmdbBackup.Name,
		UID:                psmdbBackup.UID,
		BlockOwnerDeletion: pointer.ToBool(true),
	}})

	return r.Create(ctx, backup)
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

// Reconcile PXC.
// Returns: (requeue(bool), error).
func (r *DatabaseClusterBackupReconciler) reconcilePXC(
	ctx context.Context,
	backup *everestv1alpha1.DatabaseClusterBackup,
) (bool, error) {
	pxcCR := &pxcv1.PerconaXtraDBClusterBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      backup.Name,
			Namespace: backup.Namespace,
		},
	}
	err := r.Get(ctx, types.NamespacedName{Name: backup.Name, Namespace: backup.Namespace}, pxcCR)
	if err != nil && !k8serrors.IsNotFound(err) {
		return false, err
	}

	pxcDBCR := &pxcv1.PerconaXtraDBCluster{}
	err = r.Get(ctx, types.NamespacedName{Name: backup.Spec.DBClusterName, Namespace: backup.Namespace}, pxcDBCR)
	if err != nil {
		return false, err
	}

	// Handle cleanup.
	if !backup.GetDeletionTimestamp().IsZero() {
		return true, r.handleStorageCleanup(ctx, backup, pxcCR, deletePXCBackupFinalizer)
	}

	// If the backup storage is not defined in the PerconaXtraDBCluster CR, we
	// cannot proceed
	if pxcDBCR.Spec.Backup.Storages == nil {
		return false, ErrBackupStorageUndefined
	}
	if _, ok := pxcDBCR.Spec.Backup.Storages[backup.Spec.BackupStorageName]; !ok {
		return false, ErrBackupStorageUndefined
	}

	// We don't want any backups to be deleted so let's remove the finalizer
	if controllerutil.ContainsFinalizer(pxcCR, deletePXCBackupFinalizer) {
		controllerutil.RemoveFinalizer(pxcCR, deletePXCBackupFinalizer)
		if err := r.Update(ctx, pxcCR); err != nil {
			return false, err
		}
	}

	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, pxcCR, func() error {
		pxcCR.TypeMeta = metav1.TypeMeta{
			APIVersion: pxcAPIVersion,
			Kind:       pxcBackupKind,
		}
		pxcCR.Spec.PXCCluster = backup.Spec.DBClusterName
		pxcCR.Spec.StorageName = backup.Spec.BackupStorageName

		pxcCR.SetOwnerReferences([]metav1.OwnerReference{{
			APIVersion:         everestAPIVersion,
			Kind:               databaseClusterBackupKind,
			Name:               backup.Name,
			UID:                backup.UID,
			BlockOwnerDeletion: pointer.ToBool(true),
		}})
		return nil
	})
	if err != nil {
		return false, err
	}

	backup.Status.State = everestv1alpha1.BackupState(pxcCR.Status.State)
	backup.Status.CompletedAt = pxcCR.Status.CompletedAt
	backup.Status.CreatedAt = &pxcCR.CreationTimestamp
	backup.Status.Destination = &pxcCR.Status.Destination
	for _, condition := range pxcCR.Status.Conditions {
		if condition.Reason == pxcGapsReasonString {
			backup.Status.Gaps = true
		}
	}

	return false, r.Status().Update(ctx, backup)
}

// Reconcile PSMDB.
// Returns: (requeue(bool), error).
func (r *DatabaseClusterBackupReconciler) reconcilePSMDB(
	ctx context.Context,
	backup *everestv1alpha1.DatabaseClusterBackup,
) (bool, error) {
	psmdbCR := &psmdbv1.PerconaServerMongoDBBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      backup.Name,
			Namespace: backup.Namespace,
		},
	}
	err := r.Get(ctx, types.NamespacedName{Name: backup.Name, Namespace: backup.Namespace}, psmdbCR)
	if err != nil && !k8serrors.IsNotFound(err) {
		return false, err
	}

	psmdbDBCR := &psmdbv1.PerconaServerMongoDB{}
	err = r.Get(ctx, types.NamespacedName{Name: backup.Spec.DBClusterName, Namespace: backup.Namespace}, psmdbDBCR)
	if err != nil {
		return false, err
	}

	// Handle cleanup.
	if !backup.GetDeletionTimestamp().IsZero() {
		return true, r.handleStorageCleanup(ctx, backup, psmdbCR, deletePSMDBBackupFinalizer)
	}

	// If the backup storage is not defined in the PerconaServerMongoDB CR, we
	// cannot proceed
	if psmdbDBCR.Spec.Backup.Storages == nil {
		return false, ErrBackupStorageUndefined
	}
	if _, ok := psmdbDBCR.Spec.Backup.Storages[backup.Spec.BackupStorageName]; !ok {
		return false, ErrBackupStorageUndefined
	}

	// We don't want any backups to be deleted so let's remove the finalizer
	if controllerutil.ContainsFinalizer(psmdbCR, deletePSMDBBackupFinalizer) {
		controllerutil.RemoveFinalizer(psmdbCR, deletePSMDBBackupFinalizer)
		if err := r.Update(ctx, psmdbCR); err != nil {
			return false, err
		}
		return true, nil
	}

	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, psmdbCR, func() error {
		psmdbCR.TypeMeta = metav1.TypeMeta{
			APIVersion: psmdbAPIVersion,
			Kind:       psmdbBackupKind,
		}
		psmdbCR.Spec.ClusterName = backup.Spec.DBClusterName
		psmdbCR.Spec.StorageName = backup.Spec.BackupStorageName

		psmdbCR.SetOwnerReferences([]metav1.OwnerReference{{
			APIVersion:         everestAPIVersion,
			Kind:               databaseClusterBackupKind,
			Name:               backup.Name,
			UID:                backup.UID,
			BlockOwnerDeletion: pointer.ToBool(true),
		}})
		return nil
	})
	if err != nil {
		return false, err
	}
	backup.Status.State = everestv1alpha1.BackupState(psmdbCR.Status.State)
	backup.Status.CompletedAt = psmdbCR.Status.CompletedAt
	backup.Status.CreatedAt = &psmdbCR.CreationTimestamp
	backup.Status.Destination = &psmdbCR.Status.Destination
	return false, r.Status().Update(ctx, backup)
}

// Get the last performed PG backup directly from S3.
func (r *DatabaseClusterBackupReconciler) getLastPGBackupDestination(
	ctx context.Context,
	backupStorage *everestv1alpha1.BackupStorage,
	db *everestv1alpha1.DatabaseCluster,
) *string {
	logger := log.FromContext(ctx)
	backupStorageSecret := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Name: backupStorage.Spec.CredentialsSecretName, Namespace: backupStorage.Namespace}, backupStorageSecret)
	if err != nil {
		logger.Error(err, "unable to get backup storage secret")
		return nil
	}

	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(backupStorage.Spec.Region),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			string(backupStorageSecret.Data["AWS_ACCESS_KEY_ID"]),
			string(backupStorageSecret.Data["AWS_SECRET_ACCESS_KEY"]),
			"",
		)),
	)
	if err != nil {
		logger.Error(err, "unable to load AWS configuration")
		return nil
	}
	httpClient := http.DefaultClient
	httpClient.Transport = &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: !pointer.Get(backupStorage.Spec.VerifyTLS),
		},
	}

	s3Client := s3.NewFromConfig(cfg, func(o *s3.Options) {
		o.BaseEndpoint = aws.String(backupStorage.Spec.EndpointURL)
		o.HTTPClient = httpClient
		o.UsePathStyle = pointer.Get(backupStorage.Spec.ForcePathStyle)
	})
	result, err := s3Client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
		Bucket: aws.String(backupStorage.Spec.Bucket),
		Prefix: aws.String(common.BackupStoragePrefix(db) + "/backup/db/backup.history/"),
	})
	if err != nil {
		logger.Error(err, "unable to list objects in bucket", "bucket", backupStorage.Spec.Bucket)
		return nil
	}

	if len(result.Contents) == 0 {
		logger.Error(err, "no backup found in bucket", "bucket", backupStorage.Spec.Bucket)
		return nil
	}

	var lastBackup string
	var lastModified time.Time
	for _, content := range result.Contents {
		if lastModified.Before(*content.LastModified) {
			lastModified = *content.LastModified
			lastBackup = strings.Split(filepath.Base(*content.Key), ".")[0]
		}
	}

	destination := fmt.Sprintf("s3://%s/%s/backup/db/%s", backupStorage.Spec.Bucket, common.BackupStoragePrefix(db), lastBackup)
	return &destination
}

// Reconcile PG.
// Returns: (requeue(bool), error.
func (r *DatabaseClusterBackupReconciler) reconcilePG(
	ctx context.Context,
	backup *everestv1alpha1.DatabaseClusterBackup,
) (bool, error) {
	logger := log.FromContext(ctx)

	pgCR := &pgv2.PerconaPGBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      backup.Name,
			Namespace: backup.Namespace,
		},
	}

	pgDBCR := &pgv2.PerconaPGCluster{}
	err := r.Get(ctx, types.NamespacedName{Name: backup.Spec.DBClusterName, Namespace: backup.Namespace}, pgDBCR)
	if err != nil {
		return false, err
	}

	if !backup.GetDeletionTimestamp().IsZero() {
		// We can't handle this finalizer in PG yet, so we will simply remove it (if present).
		// See: https://perconadev.atlassian.net/browse/K8SPG-538
		if controllerutil.RemoveFinalizer(backup, common.DBBBackupStorageCleanupFinalizer) {
			return true, r.Update(ctx, backup)
		}
	}

	backupStorage := &everestv1alpha1.BackupStorage{}
	err = r.Get(ctx, types.NamespacedName{Name: backup.Spec.BackupStorageName, Namespace: r.systemNamespace}, backupStorage)
	if err != nil {
		return false, errors.Join(err, fmt.Errorf("failed to get backup storage %s", backup.Spec.BackupStorageName))
	}

	// If the backup storage is not defined in the PerconaPGCluster CR, we
	// cannot proceed
	repoIdx := common.GetBackupStorageIndexInPGBackrestRepo(backupStorage, pgDBCR.Spec.Backups.PGBackRest.Repos)
	if repoIdx == -1 {
		return false, ErrBackupStorageUndefined
	}

	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, pgCR, func() error {
		pgCR.TypeMeta = metav1.TypeMeta{
			APIVersion: pgAPIVersion,
			Kind:       pgBackupKind,
		}
		pgCR.Spec.PGCluster = backup.Spec.DBClusterName
		pgCR.SetOwnerReferences([]metav1.OwnerReference{{
			APIVersion:         everestAPIVersion,
			Kind:               databaseClusterBackupKind,
			Name:               backup.Name,
			UID:                backup.UID,
			BlockOwnerDeletion: pointer.ToBool(true),
		}})

		pgCR.Spec.RepoName = pgDBCR.Spec.Backups.PGBackRest.Repos[repoIdx].Name
		pgCR.Spec.Options = []string{
			"--type=full",
		}
		return nil
	})
	if err != nil {
		return false, err
	}
	backup.Status.State = everestv1alpha1.BackupState(pgCR.Status.State)
	backup.Status.CompletedAt = pgCR.Status.CompletedAt
	backup.Status.CreatedAt = &pgCR.CreationTimestamp
	// XXX: Until https://jira.percona.com/browse/K8SPG-411 is done
	// we work around not having the destination in the
	// PerconaPGBackup CR by getting this info directly from S3
	if backup.Status.State == everestv1alpha1.BackupState(pgv2.BackupSucceeded) && backup.Status.Destination == nil {
		db := &everestv1alpha1.DatabaseCluster{}
		if err := r.Get(ctx, types.NamespacedName{
			Name:      backup.Spec.DBClusterName,
			Namespace: backup.Namespace,
		}, db); err != nil {
			logger.Error(err, "could not get database cluster ")
		}
		if err == nil {
			backup.Status.Destination = r.getLastPGBackupDestination(ctx, backupStorage, db)
		}
	}
	return false, r.Status().Update(ctx, backup)
}

func backupStorageName(repoName string, cluster *everestv1alpha1.DatabaseCluster) (string, error) {
	// repoNames in a PG cluster are in form "repo1", "repo2" etc.
	// which is mapped to the DatabaseCluster schedules list.
	// So here we figure out the BackupStorageName of the corresponding schedule.
	scheduleInd, err := strconv.Atoi(strings.TrimPrefix(repoName, "repo"))
	if err != nil {
		return "", fmt.Errorf("unable to get the schedule index for the repo %s", repoName)
	}
	// repo1 is hardcoded in the PerconaPGCluster CR as a PVC-based repo and
	// there is never a schedule for it, so there is always one less schedule
	// than repos, hence the +1. Also, the repoNames for the schedules start
	// from repo2. So we need to subtract 2 from the scheduleInd to get the
	// correct index in the schedules list.
	if len(cluster.Spec.Backup.Schedules)+1 < scheduleInd {
		return "", fmt.Errorf("invalid schedule index %v in the repo %s", scheduleInd, repoName)
	}
	return cluster.Spec.Backup.Schedules[scheduleInd-2].BackupStorageName, nil
}

// handleCleanup handles the cleanup of the DatabaseClusterBackup.
func (r *DatabaseClusterBackupReconciler) handleStorageCleanup(
	ctx context.Context,
	dbcBackup *everestv1alpha1.DatabaseClusterBackup,
	upstreamBackup client.Object,
	storageFinalizer string,
) error {
	if !controllerutil.ContainsFinalizer(dbcBackup, common.DBBBackupStorageCleanupFinalizer) {
		return nil
	}
	// Add finalizer to the upstream backup if it doesn't exist.
	if controllerutil.AddFinalizer(upstreamBackup, storageFinalizer) {
		if err := r.Update(ctx, upstreamBackup); err != nil {
			return err
		}
	}
	// Remove the everest finalizer from the DatabaseClusterBackup.
	controllerutil.RemoveFinalizer(dbcBackup, common.DBBBackupStorageCleanupFinalizer)
	if err := r.Update(ctx, dbcBackup); err != nil {
		return err
	}
	return nil
}
