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

// Package controllers contains a set of controllers for everest
package controllers

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"strings"
	"time"

	"github.com/AlekSi/pointer"
	"github.com/go-logr/logr"
	pgv2 "github.com/percona/percona-postgresql-operator/pkg/apis/pgv2.percona.com/v2"
	crunchyv1beta1 "github.com/percona/percona-postgresql-operator/pkg/apis/postgres-operator.crunchydata.com/v1beta1"
	psmdbv1 "github.com/percona/percona-server-mongodb-operator/pkg/apis/psmdb/v1"
	pxcv1 "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	everestv1alpha1 "github.com/percona/everest-operator/api/v1alpha1"
	"github.com/percona/everest-operator/internal/controller/common"
	"github.com/percona/everest-operator/internal/controller/providers"
	"github.com/percona/everest-operator/internal/controller/providers/pg"
	"github.com/percona/everest-operator/internal/controller/providers/psmdb"
	"github.com/percona/everest-operator/internal/controller/providers/pxc"
	"github.com/percona/everest-operator/internal/predicates"
)

const (
	restartAnnotationKey = "everest.percona.com/restart"

	monitoringConfigNameField        = ".spec.monitoring.monitoringConfigName"
	monitoringConfigSecretNameField  = ".spec.credentialsSecretName" //nolint:gosec
	backupStorageNameField           = ".spec.backup.schedules.backupStorageName"
	pitrBackupStorageNameField       = ".spec.backup.pitr.backupStorageName"
	credentialsSecretNameField       = ".spec.credentialsSecretName" //nolint:gosec
	podSchedulingPolicyNameField     = ".spec.podSchedulingPolicyName"
	dataSourceBackupStorageNameField = ".spec.dataSource.backupSource.backupStorageName"

	databaseClusterNameLabel = "clusterName"
	defaultRequeueAfter      = 5 * time.Second
)

var everestFinalizers = []string{
	common.UpstreamClusterCleanupFinalizer,
	common.ForegroundDeletionFinalizer,
}

// DatabaseClusterReconciler reconciles a DatabaseCluster object.
type DatabaseClusterReconciler struct {
	client.Client
	Cache  cache.Cache
	Scheme *runtime.Scheme

	controller *controllerWatcherRegistry
}

// dbProvider provides an abstraction for managing the reconciliation
// of database CRs against database operators.
type dbProvider interface {
	metav1.Object
	reconcileHooks
	Apply(ctx context.Context) everestv1alpha1.Applier
	Status(ctx context.Context) (everestv1alpha1.DatabaseClusterStatus, error)
	Cleanup(ctx context.Context, db *everestv1alpha1.DatabaseCluster) (bool, error)
	DBObject() client.Object
}

// reconcileHooks is an interface that defines the methods for the reconcile hooks.
// Each method is called at a different point in the reconcile loop.
type reconcileHooks interface {
	RunPreReconcileHook(ctx context.Context) (providers.HookResult, error)
}

// We want to make sure that our internal implementations for
// various DB operators are implementing the dbProvider interface.
var (
	_ dbProvider = (*pxc.Provider)(nil)
	_ dbProvider = (*pg.Provider)(nil)
	_ dbProvider = (*psmdb.Provider)(nil)
)

//nolint:ireturn
func (r *DatabaseClusterReconciler) newDBProvider(
	ctx context.Context,
	database *everestv1alpha1.DatabaseCluster,
) (dbProvider, error) {
	opts := providers.ProviderOptions{
		C:  r.Client,
		DB: database,
	}
	engineType := database.Spec.Engine.Type
	switch engineType {
	case everestv1alpha1.DatabaseEnginePXC:
		return pxc.New(ctx, opts)
	case everestv1alpha1.DatabaseEnginePostgresql:
		return pg.New(ctx, opts)
	case everestv1alpha1.DatabaseEnginePSMDB:
		return psmdb.New(ctx, opts)
	default:
		return nil, fmt.Errorf("unsupported engine type %s", engineType)
	}
}

//nolint:nonamedreturns
func (r *DatabaseClusterReconciler) reconcileDB(
	ctx context.Context,
	db *everestv1alpha1.DatabaseCluster,
	p dbProvider,
) (rr ctrl.Result, rerr error) {
	// Handle any necessary cleanup.
	if !db.GetDeletionTimestamp().IsZero() {
		done, err := p.Cleanup(ctx, db)
		if err != nil {
			return ctrl.Result{}, err
		}
		db.Status.Status = everestv1alpha1.AppStateDeleting
		return ctrl.Result{Requeue: !done}, r.Status().Update(ctx, db)
	}

	// Update the status of the DatabaseCluster object after the reconciliation.
	defer func() {
		status, err := p.Status(ctx)
		if err != nil {
			rr = ctrl.Result{}
			rerr = errors.Join(err, fmt.Errorf("failed to get status: %w", err))
		}
		db.Status = status
		db.Status.ObservedGeneration = db.GetGeneration()
		if err := r.Client.Status().Update(ctx, db); err != nil {
			rr = ctrl.Result{}
			rerr = errors.Join(err, fmt.Errorf("failed to update status: %w", err))
		}
		// DB is not ready, check again soon.
		if status.Status != everestv1alpha1.AppStateReady {
			rr = ctrl.Result{RequeueAfter: defaultRequeueAfter}
		}
	}()

	log := log.FromContext(ctx)
	hr, err := p.RunPreReconcileHook(ctx)
	if err != nil {
		log.Error(err, "RunPreReconcileHook failed")
		return ctrl.Result{}, err
	}
	if hr.Requeue {
		log.Info("RunPreReconcileHook requeued", "message", hr.Message)
		return ctrl.Result{Requeue: true}, nil
	}
	if hr.RequeueAfter > 0 {
		log.Info("RunPreReconcileHook requeued after", "message", hr.Message, "requeueAfter", hr.RequeueAfter)
		return ctrl.Result{RequeueAfter: hr.RequeueAfter}, nil
	}

	// Set metadata.
	p.SetName(db.GetName())
	p.SetNamespace(db.GetNamespace())
	p.SetAnnotations(db.GetAnnotations())

	// Mutate the spec and update with kube-api.
	mutate := func() error {
		applier := p.Apply(ctx)
		applier.Paused(db.Spec.Paused)
		applier.AllowUnsafeConfig()
		if err := applier.Metadata(); err != nil {
			return err
		}
		if err := applier.Engine(); err != nil {
			return err
		}
		if err := applier.Proxy(); err != nil {
			return err
		}
		if err := applier.Monitoring(); err != nil {
			return err
		}
		if err := applier.PodSchedulingPolicy(); err != nil {
			return err
		}
		if err := applier.Backup(); err != nil {
			return err
		}
		if err := applier.DataSource(); err != nil {
			return err
		}
		return nil
	}
	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, p.DBObject(), mutate); err != nil {
		return ctrl.Result{}, err
	}

	// Running the applier can possibly also mutate the DatabaseCluster,
	// so we should make sure we push those changes to the API server.
	dbCopy := db.DeepCopy()
	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, dbCopy, func() error {
		dbCopy.ObjectMeta = db.ObjectMeta
		dbCopy.Spec = db.Spec
		return nil
	}); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// +kubebuilder:rbac:groups=everest.percona.com,resources=databaseclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=everest.percona.com,resources=databaseclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=everest.percona.com,resources=databaseclusters/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch
// +kubebuilder:rbac:groups=apps,resources=statefulsets,verbs=get;list;watch
// +kubebuilder:rbac:groups=storage.k8s.io,resources=storageclasses,verbs=get;list;watch
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch
// +kubebuilder:rbac:groups=pxc.percona.com,resources=perconaxtradbclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=psmdb.percona.com,resources=perconaservermongodbs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=pgv2.percona.com,resources=perconapgclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups=everest.percona.com,resources=monitoringconfigs,verbs=get;list;watch
// +kubebuilder:rbac:groups=everest.percona.com,resources=backupstorages,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=everest.percona.com,resources=podschedulingpolicies,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.1/pkg/reconcile
func (r *DatabaseClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling", "request", req)
	defer func() {
		logger.Info("Reconciled", "request", req)
	}()

	database := &everestv1alpha1.DatabaseCluster{}
	err := r.Get(ctx, req.NamespacedName, database)
	if err != nil {
		// NotFound cannot be fixed by requeuing so ignore it. During background
		// deletion, we receive delete events from cluster's dependents after
		// cluster is deleted.
		if err = client.IgnoreNotFound(err); err != nil {
			logger.Error(err, "unable to fetch DatabaseCluster")
		}
		return reconcile.Result{}, err
	}

	if err = r.reconcileLabels(ctx, database); err != nil {
		return reconcile.Result{}, err
	}

	if err = r.handleRestart(ctx, logger, database); err != nil {
		return reconcile.Result{}, err
	}

	if err = r.ensureFinalizers(ctx, database); err != nil {
		return reconcile.Result{}, err
	}

	if database.Spec.Engine.UserSecretsName == "" {
		database.Spec.Engine.UserSecretsName = common.EverestSecretsPrefix + database.Name
	}
	if database.Spec.Engine.Replicas == 0 {
		database.Spec.Engine.Replicas = 3
	}
	if database.Spec.Proxy.Expose.Type == "" {
		database.Spec.Proxy.Expose.Type = everestv1alpha1.ExposeTypeInternal
	}

	if database.Spec.DataSource != nil &&
		database.Spec.DataSource.DBClusterBackupName != "" {
		// We don't handle database.Spec.DataSource.BackupSource in operator
		if err = r.copyCredentialsFromDBBackup(ctx, database.Spec.DataSource.DBClusterBackupName, database); err != nil {
			return reconcile.Result{}, err
		}
	}

	p, err := r.newDBProvider(ctx, database)
	if err != nil {
		return reconcile.Result{}, err
	}
	return r.reconcileDB(ctx, database, p)
}

func (r *DatabaseClusterReconciler) handleRestart(
	ctx context.Context,
	logger logr.Logger,
	database *everestv1alpha1.DatabaseCluster,
) error {
	_, restartRequired := database.ObjectMeta.Annotations[restartAnnotationKey]
	if !restartRequired {
		return nil
	}

	if !database.Spec.Paused {
		logger.Info("Pausing database cluster")
		database.Spec.Paused = true
		if err := r.Update(ctx, database); err != nil {
			return err
		}
	}
	if database.Status.Status == everestv1alpha1.AppStatePaused &&
		database.Status.Ready == 0 {
		logger.Info("Unpausing database cluster")
		database.Spec.Paused = false
		delete(database.ObjectMeta.Annotations, restartAnnotationKey)
		if err := r.Update(ctx, database); err != nil {
			return err
		}
	}
	return nil
}

// copyCredentialsFromDBBackup copies credentials from an old DB to the new DB about to be
// provisioned by providing a DB Backup name of the old DB.
func (r *DatabaseClusterReconciler) copyCredentialsFromDBBackup(
	ctx context.Context, dbBackupName string, db *everestv1alpha1.DatabaseCluster,
) error {
	logger := log.FromContext(ctx)

	dbb := &everestv1alpha1.DatabaseClusterBackup{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      dbBackupName,
		Namespace: db.Namespace,
	}, dbb)
	if err != nil {
		// It is possible that the source backup is deleted, for example, if the cluster itself was deleted.
		// If this happens, we have no way of copying the source credential secrets. So we will return from here,
		// and let the caller controller take care of failures (if any) resulting from the missing backup/secret.
		if k8serrors.IsNotFound(err) {
			return nil
		}
		return errors.Join(err, errors.New("could not get DB backup to copy credentials from old DB cluster"))
	}

	newSecretName := common.EverestSecretsPrefix + db.Name
	newSecret := &corev1.Secret{}
	err = r.Get(ctx, types.NamespacedName{
		Name:      newSecretName,
		Namespace: db.Namespace,
	}, newSecret)
	if err != nil && !k8serrors.IsNotFound(err) {
		return errors.Join(err, errors.New("could not get secret to copy credentials from old DB cluster"))
	}

	if err == nil {
		logger.Info(fmt.Sprintf("Secret %s already exists. Skipping secret copy during provisioning", newSecretName))
		return nil
	}

	prevSecretName := common.EverestSecretsPrefix + dbb.Spec.DBClusterName
	secret := &corev1.Secret{}
	err = r.Get(ctx, types.NamespacedName{
		Name:      prevSecretName,
		Namespace: db.Namespace,
	}, secret)
	if err != nil {
		return errors.Join(err, errors.New("could not get secret to copy credentials from old DB cluster"))
	}

	secret.ObjectMeta = metav1.ObjectMeta{
		Name:      newSecretName,
		Namespace: secret.Namespace,
	}
	if err := common.CreateOrUpdate(ctx, r.Client, secret, false); err != nil {
		return errors.Join(err, errors.New("could not create new secret to copy credentials from old DB cluster"))
	}

	logger.Info(fmt.Sprintf("Copied secret %s to %s", prevSecretName, newSecretName))

	return nil
}

func (r *DatabaseClusterReconciler) reconcileLabels(
	ctx context.Context, database *everestv1alpha1.DatabaseCluster,
) error {
	current := database.GetLabels()
	updated := make(map[string]string, len(current))
	maps.Copy(updated, current)

	// Remove labels for backup storage
	maps.DeleteFunc(updated, func(key string, _ string) bool {
		return strings.HasPrefix(key, "backupStorage-")
	})

	if !maps.Equal(updated, current) {
		database.SetLabels(updated)
		return r.Update(ctx, database)
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *DatabaseClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := r.initIndexers(context.Background(), mgr); err != nil {
		return err
	}

	ctrlBuilder := ctrl.NewControllerManagedBy(mgr).
		Named("DatabaseCluster").
		For(&everestv1alpha1.DatabaseCluster{})

	r.initWatchers(ctrlBuilder, common.DefaultNamespaceFilter)

	// Normally we would call `Complete()`, however, with `Build()`, we get a handle to the underlying controller,
	// so that we can dynamically add watchers from the DatabaseEngine reconciler.
	ctrl, err := ctrlBuilder.Build(r)
	if err != nil {
		return err
	}
	log := mgr.GetLogger().WithName("DynamicWatcher").WithValues("controller", "DatabaseCluster")
	r.controller = newControllerWatcherRegistry(log, ctrl)
	return nil
}

func (r *DatabaseClusterReconciler) initIndexers(ctx context.Context, mgr ctrl.Manager) error {
	// Index the BackupStorageName so that it can be used by the
	// databaseClustersThatReferenceObject function to find all
	// DatabaseClusters that reference a specific BackupStorage through the
	// BackupStorageName field
	err := mgr.GetFieldIndexer().IndexField(
		ctx, &everestv1alpha1.DatabaseCluster{}, backupStorageNameField,
		func(o client.Object) []string {
			var res []string
			database, ok := o.(*everestv1alpha1.DatabaseCluster)
			if !ok {
				return res
			}
			for _, storage := range database.Spec.Backup.Schedules {
				res = append(res, storage.BackupStorageName)
			}
			return res
		},
	)
	if err != nil {
		return err
	}

	// Index the BackupStorageName of the PITR spec so that it can be used by
	// the databaseClustersThatReferenceObject function to find all
	// DatabaseClusters that reference a specific BackupStorage through the
	// pitrBackupStorageName field
	err = mgr.GetFieldIndexer().IndexField(
		ctx, &everestv1alpha1.DatabaseCluster{}, pitrBackupStorageNameField,
		func(o client.Object) []string {
			var res []string
			database, ok := o.(*everestv1alpha1.DatabaseCluster)
			if !ok || !database.Spec.Backup.PITR.Enabled || database.Spec.Backup.PITR.BackupStorageName == nil {
				return res
			}
			return append(res, *database.Spec.Backup.PITR.BackupStorageName)
		},
	)
	if err != nil {
		return err
	}

	// Index the BackupStorageName of the .spec.dataSource.backupSource spec so that it can be used by
	// the databaseClustersThatReferenceObject function to find all
	// DatabaseClusters that reference a specific BackupStorage through the
	// dataSourceBackupStorageNameField field
	err = mgr.GetFieldIndexer().IndexField(
		ctx, &everestv1alpha1.DatabaseCluster{}, dataSourceBackupStorageNameField,
		func(o client.Object) []string {
			var res []string
			database, ok := o.(*everestv1alpha1.DatabaseCluster)
			dsBackupStoreageName := pointer.Get(pointer.Get(database.Spec.DataSource).BackupSource).BackupStorageName
			if !ok || dsBackupStoreageName == "" {
				return res
			}
			return append(res, dsBackupStoreageName)
		},
	)
	if err != nil {
		return err
	}

	// Index the monitoringConfigName field in DatabaseCluster.
	err = mgr.GetFieldIndexer().IndexField(
		ctx, &everestv1alpha1.DatabaseCluster{}, monitoringConfigNameField,
		func(o client.Object) []string {
			var res []string
			dc, ok := o.(*everestv1alpha1.DatabaseCluster)
			if !ok {
				return res
			}
			if dc.Spec.Monitoring != nil {
				res = append(res, dc.Spec.Monitoring.MonitoringConfigName)
			}
			return res
		},
	)
	if err != nil {
		return err
	}

	// Index the podSchedulingPolicyName field in DatabaseCluster.
	err = mgr.GetFieldIndexer().IndexField(
		ctx, &everestv1alpha1.DatabaseCluster{}, podSchedulingPolicyNameField,
		func(o client.Object) []string {
			var res []string
			dc, ok := o.(*everestv1alpha1.DatabaseCluster)
			if !ok {
				return res
			}

			if dc.Spec.PodSchedulingPolicyName != "" {
				res = append(res, dc.Spec.PodSchedulingPolicyName)
			}
			return res
		},
	)
	if err != nil {
		return err
	}

	return err
}

func (r *DatabaseClusterReconciler) initWatchers(controller *builder.Builder, defaultPredicate predicate.Predicate) { //nolint:gocognit
	controller.Watches(
		&corev1.Namespace{},
		common.EnqueueObjectsInNamespace(r.Client, &everestv1alpha1.DatabaseClusterList{}),
		builder.WithPredicates(defaultPredicate),
	)

	controller.Watches(
		&everestv1alpha1.BackupStorage{},
		handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
			bs, ok := obj.(*everestv1alpha1.BackupStorage)
			if !ok {
				return []reconcile.Request{}
			}

			// use map to avoid duplicates of DatabaseCluster name+namespace pairs
			dbsToReconcileMap := make(map[types.NamespacedName]struct{})

			// Find all DatabaseClusters that reference the BackupStorage
			// through the BackupStorageName field
			attachedDBs, err := common.DatabaseClustersThatReferenceObject(ctx, r.Client,
				backupStorageNameField, bs.GetNamespace(), bs.GetName())
			if err != nil {
				return []reconcile.Request{}
			}
			for _, db := range attachedDBs.Items {
				dbsToReconcileMap[types.NamespacedName{
					Name:      db.GetName(),
					Namespace: db.GetNamespace(),
				}] = struct{}{}
			}

			// Find all DatabaseClusters that reference the BackupStorage
			// through the PITRBackupStorageName field
			attachedDBs, err = common.DatabaseClustersThatReferenceObject(ctx, r.Client,
				pitrBackupStorageNameField, bs.GetNamespace(), bs.GetName())
			if err != nil {
				return []reconcile.Request{}
			}
			for _, db := range attachedDBs.Items {
				dbsToReconcileMap[types.NamespacedName{
					Name:      db.GetName(),
					Namespace: db.GetNamespace(),
				}] = struct{}{}
			}

			// Find all DatabaseClusters that are referenced by
			// DatabaseClusterBackups that reference the BackupStorage
			attachedDBBs, err := common.DatabaseClusterBackupsThatReferenceObject(ctx, r.Client,
				common.DBClusterBackupBackupStorageNameField, bs.GetNamespace(), bs.GetName())
			if err != nil {
				return []reconcile.Request{}
			}
			for _, dbb := range attachedDBBs.Items {
				dbsToReconcileMap[types.NamespacedName{
					Name:      dbb.Spec.DBClusterName,
					Namespace: dbb.GetNamespace(),
				}] = struct{}{}
			}

			requests := make([]reconcile.Request, len(dbsToReconcileMap))
			i := 0
			for db := range dbsToReconcileMap {
				requests[i] = reconcile.Request{
					NamespacedName: db,
				}
				i++
			}

			return requests
		}),
		builder.WithPredicates(predicate.GenerationChangedPredicate{},
			predicates.GetBackupStoragePredicate(),
			defaultPredicate,
		),
	)

	// We watch DBEngines since they contain the result of the operator upgrades.
	// We subscribe to the operator upgrades so that we're able to update status/metadata on the
	// cluster, such as CRVersion and RecommendedCRVersion.
	controller.Watches(
		&everestv1alpha1.DatabaseEngine{},
		handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
			return r.databaseClustersInObjectNamespace(ctx, obj)
		}),
		builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}, defaultPredicate),
	)

	controller.Watches(
		&everestv1alpha1.MonitoringConfig{},
		handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
			mc, ok := obj.(*everestv1alpha1.MonitoringConfig)
			if !ok {
				return []reconcile.Request{}
			}

			attachedDatabaseClusters, err := common.DatabaseClustersThatReferenceObject(ctx, r.Client,
				monitoringConfigNameField, mc.GetNamespace(), mc.GetName())
			if err != nil {
				return []reconcile.Request{}
			}

			requests := make([]reconcile.Request, len(attachedDatabaseClusters.Items))
			for i, item := range attachedDatabaseClusters.Items {
				requests[i] = reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      item.GetName(),
						Namespace: item.GetNamespace(),
					},
				}
			}

			return requests
		}),
		builder.WithPredicates(predicate.GenerationChangedPredicate{},
			predicates.GetMonitoringConfigPredicate(),
			defaultPredicate),
	)

	controller.Watches(
		&everestv1alpha1.PodSchedulingPolicy{},
		handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
			psp, ok := obj.(*everestv1alpha1.PodSchedulingPolicy)
			if !ok {
				return []reconcile.Request{}
			}

			attachedDatabaseClusters, err := common.DatabaseClustersThatReferenceObject(ctx, r.Client,
				podSchedulingPolicyNameField, "", psp.GetName())
			if err != nil {
				return []reconcile.Request{}
			}

			requests := make([]reconcile.Request, len(attachedDatabaseClusters.Items))
			for i, item := range attachedDatabaseClusters.Items {
				requests[i] = reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      item.GetName(),
						Namespace: item.GetNamespace(),
					},
				}
			}

			return requests
		}),
		builder.WithPredicates(predicate.GenerationChangedPredicate{},
			predicates.GetPodSchedulingPolicyPredicate()),
		// defaultPredicate is not needed here since PodSchedulingPolicy doesn't belong to any namespace.
	)

	// In PG reconciliation we create a backup credentials secret because the
	// PG operator requires this secret to be encoded differently from the
	// generic one used in PXC and PSMDB. Therefore, we need to watch for
	// secrets, specifically the ones that are referenced in DatabaseCluster
	// CRs, and trigger a reconciliation if these change so that we can
	// reenconde the secret required by PG.
	controller.Owns(&corev1.Secret{})
	controller.Watches(
		&corev1.Secret{},
		handler.EnqueueRequestsFromMapFunc(r.databaseClustersThatReferenceSecret),
		builder.WithPredicates(predicate.GenerationChangedPredicate{}, defaultPredicate),
	)

	controller.Watches(
		&everestv1alpha1.DatabaseClusterBackup{},
		handler.EnqueueRequestsFromMapFunc(func(_ context.Context, obj client.Object) []reconcile.Request {
			dbClusterBackup, ok := obj.(*everestv1alpha1.DatabaseClusterBackup)
			if !ok {
				return []reconcile.Request{}
			}
			return []reconcile.Request{
				{
					NamespacedName: types.NamespacedName{
						Name:      dbClusterBackup.Spec.DBClusterName,
						Namespace: obj.GetNamespace(),
					},
				},
			}
		}),
		builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}, defaultPredicate),
	)

	controller.Watches(
		&everestv1alpha1.DatabaseClusterRestore{},
		handler.EnqueueRequestsFromMapFunc(func(_ context.Context, obj client.Object) []reconcile.Request {
			dbClusterRestore, ok := obj.(*everestv1alpha1.DatabaseClusterRestore)
			if !ok {
				return []reconcile.Request{}
			}
			return []reconcile.Request{
				{
					NamespacedName: types.NamespacedName{
						Name:      dbClusterRestore.Spec.DBClusterName,
						Namespace: obj.GetNamespace(),
					},
				},
			}
		}),
		builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}, defaultPredicate),
	)
}

func (r *DatabaseClusterReconciler) databaseClustersInObjectNamespace(ctx context.Context, obj client.Object) []reconcile.Request {
	dbs := &everestv1alpha1.DatabaseClusterList{}
	err := r.List(ctx, dbs, client.InNamespace(obj.GetNamespace()))
	if err != nil {
		return nil
	}

	result := make([]reconcile.Request, 0, len(dbs.Items))
	for _, db := range dbs.Items {
		result = append(result, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      db.GetName(),
				Namespace: db.GetNamespace(),
			},
		})
	}
	return result
}

// databaseClustersThatReferenceSecret returns a list of reconcile
// requests for all DatabaseClusters that reference the given secret.
func (r *DatabaseClusterReconciler) databaseClustersThatReferenceSecret(ctx context.Context, secret client.Object) []reconcile.Request {
	logger := log.FromContext(ctx)

	// BackupStorage
	if bsList, err := common.BackupStoragesThatReferenceObject(ctx, r.Client,
		credentialsSecretNameField, secret.GetNamespace(), secret.GetName()); err != nil {
		logger.Error(err, fmt.Sprintf("could not find BackupStorages by secret name='%s' in namespace='%s'", secret.GetName(), secret.GetNamespace()))
	} else {
		objs := make([]client.Object, len(bsList.Items))
		for i, item := range bsList.Items {
			// With the move to go 1.22 it's safe to reuse the same variable,
			// see https://go.dev/blog/loopvar-preview. However, exportloopref
			// linter doesn't like it. Let's disable them for this line until
			// they are updated to support go 1.22.
			objs[i] = &item //nolint:exportloopref
		}
		return r.getDBClustersReconcileRequestsByRelatedObjectName(ctx, objs, backupStorageNameField)
	}

	// MonitoringConfig
	if mcList, err := common.MonitoringConfigsThatReferenceObject(ctx, r.Client,
		monitoringConfigSecretNameField, secret.GetNamespace(), secret.GetName()); err != nil {
		logger.Error(err, fmt.Sprintf("could not find MonitoringConfigs by secret name='%s' in namespace='%s'", secret.GetName(), secret.GetNamespace()))
	} else {
		objs := make([]client.Object, len(mcList.Items))
		for i, item := range mcList.Items {
			// With the move to go 1.22 it's safe to reuse the same variable,
			// see https://go.dev/blog/loopvar-preview. However, exportloopref
			// linter doesn't like it. Let's disable them for this line until
			// they are updated to support go 1.22.
			objs[i] = &item //nolint:exportloopref
		}
		return r.getDBClustersReconcileRequestsByRelatedObjectName(ctx, objs, monitoringConfigNameField)
	}

	return nil
}

func (r *DatabaseClusterReconciler) getDBClustersReconcileRequestsByRelatedObjectName(ctx context.Context, items []client.Object, fieldPath string) []reconcile.Request {
	logger := log.FromContext(ctx)

	var requests []reconcile.Request
	for _, i := range items {
		attachedDatabaseClusters, err := common.DatabaseClustersThatReferenceObject(ctx, r.Client, fieldPath, i.GetNamespace(), i.GetName())
		if err != nil {
			logger.Error(err, fmt.Sprintf("could not find DatabaseClusters by '%s'='%s' in namespace='%s'", fieldPath, i.GetName(), i.GetNamespace()))
			return []reconcile.Request{}
		}

		for _, item := range attachedDatabaseClusters.Items {
			request := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      item.GetName(),
					Namespace: item.GetNamespace(),
				},
			}
			requests = append(requests, request)
		}
	}

	return requests
}

func (r *DatabaseClusterReconciler) ensureFinalizers(
	ctx context.Context,
	database *everestv1alpha1.DatabaseCluster,
) error {
	if !database.DeletionTimestamp.IsZero() {
		return nil
	}

	var updated bool
	for _, f := range everestFinalizers {
		if controllerutil.AddFinalizer(database, f) {
			updated = true
		}
	}

	if updated {
		return r.Client.Update(ctx, database)
	}
	return nil
}

// ReconcileWatchers reconciles the watchers for the DatabaseCluster controller.
func (r *DatabaseClusterReconciler) ReconcileWatchers(ctx context.Context) error {
	dbEngines := &everestv1alpha1.DatabaseEngineList{}
	if err := r.List(ctx, dbEngines); err != nil {
		return err
	}

	log := log.FromContext(ctx)
	addWatcher := func(dbEngineType everestv1alpha1.EngineType, obj client.Object) error {
		sources := []source.Source{
			source.Kind(r.Cache, obj, &handler.EnqueueRequestForObject{}),
		}

		// special case for PXC - we need to watch pxc-restore to be sure the db is reconciled on every pxc-restore status update.
		// watching the dbr is not enough since the operator merges the statuses but we need to pause the db exactly when
		// the pxc-restore got to the pxcv1.RestoreStopCluster status
		if dbEngineType == everestv1alpha1.DatabaseEnginePXC {
			sources = append(sources, newPXCRestoreWatchSource(r.Cache))
		}

		// Since PerconaPGCluster does not expose any info about volume resizing,
		// we need to directly watch the PostgresCluster objects to track the status.
		// See: https://perconadev.atlassian.net/browse/K8SPG-748
		// TODO: Remove this once K8SPG-748 is addressed.
		if dbEngineType == everestv1alpha1.DatabaseEnginePostgresql {
			sources = append(sources, newCrunchyWatchSource(r.Cache))
		}

		if err := r.controller.addWatchers(string(dbEngineType), sources...); err != nil {
			return err
		}
		return nil
	}

	for _, dbEngine := range dbEngines.Items {
		if dbEngine.Status.State != everestv1alpha1.DBEngineStateInstalled {
			continue
		}
		switch t := dbEngine.Spec.Type; t {
		case everestv1alpha1.DatabaseEnginePXC:
			if err := addWatcher(t, &pxcv1.PerconaXtraDBCluster{}); err != nil {
				return err
			}
		case everestv1alpha1.DatabaseEnginePostgresql:
			if err := addWatcher(t, &pgv2.PerconaPGCluster{}); err != nil {
				return err
			}
		case everestv1alpha1.DatabaseEnginePSMDB:
			if err := addWatcher(t, &psmdbv1.PerconaServerMongoDB{}); err != nil {
				return err
			}
		default:
			log.Info("Unknown database engine type", "type", dbEngine.Spec.Type)
			continue
		}
	}
	return nil
}

func newPXCRestoreWatchSource(cache cache.Cache) source.Source { //nolint:ireturn
	return source.TypedKind[client.Object](cache, &pxcv1.PerconaXtraDBClusterRestore{},
		handler.EnqueueRequestsFromMapFunc(func(_ context.Context, obj client.Object) []reconcile.Request {
			pxcRestore, ok := obj.(*pxcv1.PerconaXtraDBClusterRestore)
			if !ok {
				return []reconcile.Request{}
			}
			return []reconcile.Request{
				{
					NamespacedName: types.NamespacedName{
						Name:      pxcRestore.Spec.PXCCluster,
						Namespace: obj.GetNamespace(),
					},
				},
			}
		}),
		predicate.ResourceVersionChangedPredicate{},
		common.DefaultNamespaceFilter,
	)
}

func newCrunchyWatchSource(cache cache.Cache) source.Source { //nolint:ireturn
	return source.TypedKind[client.Object](cache, &crunchyv1beta1.PostgresCluster{},
		&handler.EnqueueRequestForObject{},
		// We are watching PostgresCluster objects since PerconaPGCluster lacks status
		// info about volume resizing. So we will attach an event filter to only trigger
		// reconciliations when the volume is being resized.
		predicate.NewPredicateFuncs(func(obj client.Object) bool {
			pgc, ok := obj.(*crunchyv1beta1.PostgresCluster)
			if !ok {
				return false
			}
			return meta.IsStatusConditionTrue(pgc.Status.Conditions, crunchyv1beta1.PersistentVolumeResizing)
		}),
		common.DefaultNamespaceFilter,
	)
}
