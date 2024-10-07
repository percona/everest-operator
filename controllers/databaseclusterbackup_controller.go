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
	"k8s.io/client-go/util/workqueue"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	everestv1alpha1 "github.com/percona/everest-operator/api/v1alpha1"
	"github.com/percona/everest-operator/controllers/common"
)

const (
	databaseClusterKind       = "DatabaseCluster"
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
	Scheme *runtime.Scheme
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

	// DBBackups are always deleted in foreground.
	if backup.GetDeletionTimestamp().IsZero() &&
		controllerutil.AddFinalizer(backup, common.ForegroundDeletionFinalizer) {
		if err := r.Update(ctx, backup); err != nil {
			return reconcile.Result{}, err
		}
		return reconcile.Result{Requeue: true}, nil // Requeue so that we get an updated copy.
	}

	cluster := &everestv1alpha1.DatabaseCluster{}
	err = r.Get(ctx, types.NamespacedName{Name: backup.Spec.DBClusterName, Namespace: backup.Namespace}, cluster)
	if err != nil {
		if err = client.IgnoreNotFound(err); err != nil {
			logger.Error(err, "unable to fetch DatabaseCluster")
		}
		return reconcile.Result{}, err
	}

	if err := r.reconcileMeta(ctx, backup, cluster); err != nil {
		return reconcile.Result{}, err
	}

	storage := &everestv1alpha1.BackupStorage{}
	err = r.Get(ctx, types.NamespacedName{
		Name:      backup.Spec.BackupStorageName,
		Namespace: backup.GetNamespace(),
	}, storage)
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

func (r *DatabaseClusterBackupReconciler) reconcileMeta(
	ctx context.Context,
	backup *everestv1alpha1.DatabaseClusterBackup,
	cluster *everestv1alpha1.DatabaseCluster,
) error {
	var needUpdate bool
	if len(backup.ObjectMeta.Labels) == 0 {
		backup.ObjectMeta.Labels = map[string]string{
			databaseClusterNameLabel: backup.Spec.DBClusterName,
			fmt.Sprintf(backupStorageNameLabelTmpl, backup.Spec.BackupStorageName): backupStorageLabelValue,
		}
		needUpdate = true
	}
	if metav1.GetControllerOf(backup) == nil {
		if err := controllerutil.SetControllerReference(cluster, backup, r.Client.Scheme()); err != nil {
			return err
		}
		needUpdate = true
	}

	if needUpdate {
		return r.Update(ctx, backup)
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DatabaseClusterBackupReconciler) SetupWithManager(mgr ctrl.Manager) error {
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
	controller.Watches(
		&corev1.Namespace{},
		common.EnqueueObjectsInNamespace(r.Client, &everestv1alpha1.DatabaseClusterBackupList{}),
	)
	ctx := context.Background()

	err = r.Get(ctx, types.NamespacedName{Name: pxcBackupCRDName}, unstructuredResource)
	if err == nil {
		if err := r.addPXCToScheme(r.Scheme); err == nil {
			controller.Watches(
				&pxcv1.PerconaXtraDBClusterBackup{},
				r.watchHandler(r.tryCreatePXC),
			)
		}
	}

	err = r.Get(ctx, types.NamespacedName{Name: psmdbBackupCRDName}, unstructuredResource)
	if err == nil {
		if err := r.addPSMDBToScheme(r.Scheme); err == nil {
			controller.Watches(
				&psmdbv1.PerconaServerMongoDBBackup{},
				r.watchHandler(r.tryCreatePSMDB),
			)
		}
	}

	err = r.Get(ctx, types.NamespacedName{Name: pgBackupCRDName}, unstructuredResource)
	if err == nil {
		if err := r.addPGToScheme(r.Scheme); err == nil {
			controller.Watches(
				&pgv2.PerconaPGBackup{},
				r.watchHandler(r.tryCreatePG),
			)
		}
	}

	controller.WithEventFilter(common.DefaultNamespaceFilter)
	return controller.Complete(r)
}

func (r *DatabaseClusterBackupReconciler) watchHandler(creationFunc func(ctx context.Context, obj client.Object) error) handler.Funcs {
	return handler.Funcs{
		CreateFunc: func(ctx context.Context, e event.CreateEvent, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
			r.tryCreateDBBackups(ctx, e.Object, creationFunc)
			q.Add(reconcileRequestFromObject(e.Object))
		},
		UpdateFunc: func(ctx context.Context, e event.UpdateEvent, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
			// remove the r.tryCreateDBBackups call below once https://perconadev.atlassian.net/browse/K8SPSMDB-1088 is fixed.
			r.tryCreateDBBackups(ctx, e.ObjectNew, creationFunc)

			q.Add(reconcileRequestFromObject(e.ObjectNew))
		},
		DeleteFunc: func(ctx context.Context, e event.DeleteEvent, q workqueue.TypedRateLimitingInterface[reconcile.Request]) {
			r.tryDeleteDBBackup(ctx, e.Object)
			q.Add(reconcileRequestFromObject(e.Object))
		},
	}
}

func reconcileRequestFromObject(obj client.Object) ctrl.Request {
	return ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      obj.GetName(),
			Namespace: obj.GetNamespace(),
		},
	}
}

func (r *DatabaseClusterBackupReconciler) tryDeleteDBBackup(ctx context.Context, obj client.Object) {
	logger := log.FromContext(ctx)
	dbb := &everestv1alpha1.DatabaseClusterBackup{}
	namespacedName := types.NamespacedName{
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}

	if err := r.Get(ctx, namespacedName, dbb); err != nil {
		if k8serrors.IsNotFound(err) {
			return
		}
		logger.Error(err, "Failed to get the DatabaseClusterBackup", "name", obj.GetName())
		return
	}

	if err := r.Delete(ctx, dbb); err != nil {
		if k8serrors.IsNotFound(err) {
			return
		}
		logger.Error(err, "Failed to delete the DatabaseClusterBackup", "name", obj.GetName())
	}
}

func (r *DatabaseClusterBackupReconciler) tryCreateDBBackups(
	ctx context.Context,
	obj client.Object,
	createBackupFunc func(ctx context.Context, obj client.Object) error,
) {
	logger := log.FromContext(ctx)
	if len(obj.GetOwnerReferences()) == 0 {
		err := createBackupFunc(ctx, obj)
		if err != nil {
			logger.Error(err, "Failed to create DatabaseClusterBackup "+obj.GetName())
		}
	}
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

	pg := &pgv2.PerconaPGCluster{}
	if err := r.Get(ctx, types.NamespacedName{Namespace: obj.GetNamespace(), Name: pgBackup.Spec.PGCluster}, pg); err != nil {
		// if such upstream cluster is not found - do nothing
		if k8serrors.IsNotFound(err) {
			return nil
		}
		return err
	}

	storages := &everestv1alpha1.BackupStorageList{}
	if err := r.List(ctx, storages, &client.ListOptions{}); err != nil {
		// if no backup storages found - do nothing
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

	backup.Spec.DBClusterName = pgBackup.Spec.PGCluster

	cluster := &everestv1alpha1.DatabaseCluster{}
	err = r.Get(ctx, types.NamespacedName{Name: pgBackup.Spec.PGCluster, Namespace: pgBackup.Namespace}, cluster)
	if err != nil {
		return err
	}
	name, nErr := backupStorageName(pgBackup.Spec.RepoName, pg, storages)
	if nErr != nil {
		return nErr
	}

	backup.Spec.BackupStorageName = name
	backup.ObjectMeta.Labels = map[string]string{
		databaseClusterNameLabel:                      pgBackup.Spec.PGCluster,
		fmt.Sprintf(backupStorageNameLabelTmpl, name): backupStorageLabelValue,
	}
	if err = controllerutil.SetControllerReference(cluster, backup, r.Scheme); err != nil {
		return err
	}

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
	cluster := &everestv1alpha1.DatabaseCluster{}
	err = r.Get(ctx, types.NamespacedName{Name: pxcBackup.Spec.PXCCluster, Namespace: pxcBackup.Namespace}, cluster)
	if err != nil {
		return err
	}
	if err = controllerutil.SetControllerReference(cluster, backup, r.Scheme); err != nil {
		return err
	}

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
	cluster := &everestv1alpha1.DatabaseCluster{}
	err = r.Get(ctx, types.NamespacedName{Name: psmdbBackup.Spec.ClusterName, Namespace: psmdbBackup.Namespace}, cluster)
	if err != nil {
		return err
	}
	if err = controllerutil.SetControllerReference(cluster, backup, r.Scheme); err != nil {
		return err
	}
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
	if err := r.Get(ctx,
		types.NamespacedName{
			Name:      backup.Name,
			Namespace: backup.Namespace,
		},
		pxcCR); client.IgnoreNotFound(err) != nil {
		return false, err
	}

	pxcDBCR := &pxcv1.PerconaXtraDBCluster{}
	err := r.Get(ctx, types.NamespacedName{Name: backup.Spec.DBClusterName, Namespace: backup.Namespace}, pxcDBCR)
	if err != nil {
		return false, err
	}

	// Handle cleanup.
	if !backup.GetDeletionTimestamp().IsZero() {
		if err := r.handleStorageProtectionFinalizer(ctx, backup, pxcCR, deletePXCBackupFinalizer); err != nil {
			return false, err
		}
		backup.Status.State = everestv1alpha1.BackupDeleting
		return true, r.Status().Update(ctx, backup)
	}

	// If the backup storage is not defined in the PerconaXtraDBCluster CR, we
	// cannot proceed
	if pxcDBCR.Spec.Backup.Storages == nil {
		return false, ErrBackupStorageUndefined
	}
	if _, ok := pxcDBCR.Spec.Backup.Storages[backup.Spec.BackupStorageName]; !ok {
		return false, ErrBackupStorageUndefined
	}

	if !backup.HasCompleted() {
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
			controllerutil.AddFinalizer(pxcCR, deletePXCBackupFinalizer)
			return nil
		})
		if err != nil {
			return false, err
		}
	}

	backup.Status.State = everestv1alpha1.GetDBBackupState(pxcCR)
	backup.Status.CompletedAt = pxcCR.Status.CompletedAt
	backup.Status.CreatedAt = &pxcCR.CreationTimestamp
	backup.Status.Destination = pointer.To(string(pxcCR.Status.Destination))
	for _, condition := range pxcCR.Status.Conditions {
		if condition.Reason == pxcGapsReasonString {
			backup.Status.Gaps = true
		}
	}
	backup.Status.LatestRestorableTime = pxcCR.Status.LatestRestorableTime
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
	if err := r.Get(ctx, types.NamespacedName{
		Name:      backup.Name,
		Namespace: backup.Namespace,
	},
		psmdbCR); client.IgnoreNotFound(err) != nil {
		return false, err
	}

	// If the psmdb-backup object exists, we will wait for it to progress beyond the waiting state.
	// This is a known limitation in PSMSD operator, where updating the object while it is in the waiting
	// state results in a duplicate backup being created.
	// See: https://perconadev.atlassian.net/browse/K8SPSMDB-1088
	if !pointer.To(psmdbCR.GetCreationTimestamp()).IsZero() &&
		(psmdbCR.Status.State == "" || psmdbCR.Status.State == psmdbv1.BackupStateWaiting) {
		return true, nil
	}

	psmdbDBCR := &psmdbv1.PerconaServerMongoDB{}
	err := r.Get(ctx, types.NamespacedName{Name: backup.Spec.DBClusterName, Namespace: backup.Namespace}, psmdbDBCR)
	if err != nil {
		return false, err
	}

	// Handle cleanup.
	if !backup.GetDeletionTimestamp().IsZero() {
		if err := r.handleStorageProtectionFinalizer(ctx, backup, psmdbCR, deletePSMDBBackupFinalizer); err != nil {
			return false, err
		}
		backup.Status.State = everestv1alpha1.BackupDeleting
		return true, r.Status().Update(ctx, backup)
	}

	// If the backup storage is not defined in the PerconaServerMongoDB CR, we
	// cannot proceed
	if psmdbDBCR.Spec.Backup.Storages == nil {
		return false, ErrBackupStorageUndefined
	}
	if _, ok := psmdbDBCR.Spec.Backup.Storages[backup.Spec.BackupStorageName]; !ok {
		return false, ErrBackupStorageUndefined
	}

	if !backup.HasCompleted() {
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
			controllerutil.AddFinalizer(psmdbCR, deletePSMDBBackupFinalizer)
			return nil
		})
		if err != nil {
			return false, err
		}
	}
	backup.Status.State = everestv1alpha1.GetDBBackupState(psmdbCR)
	backup.Status.CompletedAt = psmdbCR.Status.CompletedAt
	backup.Status.CreatedAt = &psmdbCR.CreationTimestamp
	backup.Status.Destination = &psmdbCR.Status.Destination
	backup.Status.LatestRestorableTime = psmdbCR.Status.LatestRestorableTime
	return false, r.Status().Update(ctx, backup)
}

// Get the last performed PG backup directly from S3.
func (r *DatabaseClusterBackupReconciler) getLastPGBackupDestination(
	ctx context.Context,
	backupStorage *everestv1alpha1.BackupStorage,
	db *everestv1alpha1.DatabaseCluster,
	pgBackup *pgv2.PerconaPGBackup,
) *string {
	logger := log.FromContext(ctx)
	if pgBackup.Status.BackupName != "" {
		logger.Info("using backup name from status")
		destination := fmt.Sprintf("s3://%s/%s/backup/db/%s", backupStorage.Spec.Bucket, common.BackupStoragePrefix(db), pgBackup.Status.BackupName)
		return &destination
	}
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
			InsecureSkipVerify: !pointer.Get(backupStorage.Spec.VerifyTLS), //nolint:gosec
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
	if err := r.Get(ctx, types.NamespacedName{
		Name:      backup.GetName(),
		Namespace: backup.GetNamespace(),
	},
		pgCR); client.IgnoreNotFound(err) != nil {
		return false, err
	}

	pgDBCR := &pgv2.PerconaPGCluster{}
	err := r.Get(ctx, types.NamespacedName{Name: backup.Spec.DBClusterName, Namespace: backup.Namespace}, pgDBCR)
	if err != nil {
		return false, err
	}

	if !backup.GetDeletionTimestamp().IsZero() {
		// We can't handle this finalizer in PG yet, so we will simply remove it (if present).
		// See: https://perconadev.atlassian.net/browse/K8SPG-538
		if controllerutil.RemoveFinalizer(backup, everestv1alpha1.DBBackupStorageProtectionFinalizer) {
			return true, r.Update(ctx, backup)
		}
		backup.Status.State = everestv1alpha1.BackupDeleting
		return true, r.Status().Update(ctx, backup)
	}

	backupStorage := &everestv1alpha1.BackupStorage{}
	err = r.Get(ctx, types.NamespacedName{
		Name:      backup.Spec.BackupStorageName,
		Namespace: backup.GetNamespace(),
	}, backupStorage)
	if err != nil {
		return false, errors.Join(err, fmt.Errorf("failed to get backup storage %s", backup.Spec.BackupStorageName))
	}

	// If the backup storage is not defined in the PerconaPGCluster CR, we
	// cannot proceed
	repoName := common.GetRepoNameByBackupStorage(backupStorage, pgDBCR.Spec.Backups.PGBackRest.Repos)
	if repoName == "" {
		return false, ErrBackupStorageUndefined
	}

	if !backup.HasCompleted() {
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

			pgCR.Spec.RepoName = repoName
			pgCR.Spec.Options = []string{
				"--type=full",
			}
			return nil
		})
		if err != nil {
			return false, err
		}
	}
	backup.Status.State = everestv1alpha1.GetDBBackupState(pgCR)
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
			backup.Status.Destination = r.getLastPGBackupDestination(ctx, backupStorage, db, pgCR)
		}
	}
	backup.Status.LatestRestorableTime = pgCR.Status.LatestRestorableTime.Time
	return false, r.Status().Update(ctx, backup)
}

func backupStorageName(repoName string, pg *pgv2.PerconaPGCluster, storages *everestv1alpha1.BackupStorageList) (string, error) {
	for _, repo := range pg.Spec.Backups.PGBackRest.Repos {
		if repo.Name == repoName {
			for _, storage := range storages.Items {
				if pg.Namespace == storage.Namespace &&
					repo.S3.Region == storage.Spec.Region &&
					repo.S3.Bucket == storage.Spec.Bucket &&
					repo.S3.Endpoint == storage.Spec.EndpointURL {
					return storage.Name, nil
				}
			}
		}
	}
	return "", fmt.Errorf("failed to find backup storage for repo %s", repoName)
}

func (r *DatabaseClusterBackupReconciler) handleStorageProtectionFinalizer(
	ctx context.Context,
	dbcBackup *everestv1alpha1.DatabaseClusterBackup,
	upstreamBackup client.Object,
	storageFinalizer string,
) error {
	if !controllerutil.ContainsFinalizer(dbcBackup, everestv1alpha1.DBBackupStorageProtectionFinalizer) {
		return nil
	}
	// Ensure that S3 finalizer is removed from the upstream backup.
	if controllerutil.RemoveFinalizer(upstreamBackup, storageFinalizer) {
		return r.Update(ctx, upstreamBackup)
	}
	// Finalizer is gone from upstream object, remove from DatabaseClusterBackup.
	if controllerutil.RemoveFinalizer(dbcBackup, everestv1alpha1.DBBackupStorageProtectionFinalizer) {
		return r.Update(ctx, dbcBackup)
	}
	return nil
}
