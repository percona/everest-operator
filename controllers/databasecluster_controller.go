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
	"bytes"
	"container/list"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/AlekSi/pointer"
	"github.com/go-ini/ini"
	pgv2 "github.com/percona/percona-postgresql-operator/pkg/apis/pgv2.percona.com/v2"
	crunchyv1beta1 "github.com/percona/percona-postgresql-operator/pkg/apis/postgres-operator.crunchydata.com/v1beta1"
	psmdbv1 "github.com/percona/percona-server-mongodb-operator/pkg/apis/psmdb/v1"
	"github.com/percona/percona-server-mongodb-operator/pkg/util/numstr"
	pxcv1 "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	everestv1alpha1 "github.com/percona/everest-operator/api/v1alpha1"
)

// ClusterType used to understand the underlying platform of k8s cluster.
type ClusterType string

const (
	// PerconaXtraDBClusterKind represents pxc kind.
	PerconaXtraDBClusterKind = "PerconaXtraDBCluster"
	// PerconaServerMongoDBKind represents psmdb kind.
	PerconaServerMongoDBKind = "PerconaServerMongoDB"
	// PerconaPGClusterKind represents postgresql kind.
	PerconaPGClusterKind = "PerconaPGCluster"

	pxcDeploymentName       = "percona-xtradb-cluster-operator"
	pxcHAProxyEnvSecretName = "haproxy-env-secret" //nolint:gosec // This is not a credential, only a secret name.
	psmdbDeploymentName     = "percona-server-mongodb-operator"
	pgDeploymentName        = "percona-postgresql-operator"

	defaultPMMClientImage = "percona/pmm-client:2"

	psmdbCRDName                = "perconaservermongodbs.psmdb.percona.com"
	pxcCRDName                  = "perconaxtradbclusters.pxc.percona.com"
	pgCRDName                   = "perconapgclusters.pgv2.percona.com"
	pxcAPIGroup                 = "pxc.percona.com"
	psmdbAPIGroup               = "psmdb.percona.com"
	pgAPIGroup                  = "pgv2.percona.com"
	haProxyTemplate             = "percona/percona-xtradb-cluster-operator:%s-haproxy"
	restartAnnotationKey        = "everest.percona.com/restart"
	dbTemplateKindAnnotationKey = "everest.percona.com/dbtemplate-kind"
	dbTemplateNameAnnotationKey = "everest.percona.com/dbtemplate-name"
	// ClusterTypeEKS represents EKS cluster type.
	ClusterTypeEKS ClusterType = "eks"
	// ClusterTypeMinikube represents minikube cluster type.
	ClusterTypeMinikube ClusterType = "minikube"

	psmdbDefaultConfigurationTemplate = `
      operationProfiling:
        mode: slowOp
`
	monitoringConfigNameField       = ".spec.monitoring.monitoringConfigName"
	monitoringConfigSecretNameField = ".spec.credentialsSecretName" //nolint:gosec
	backupStorageNameField          = ".spec.backup.schedules.backupStorageName"
	credentialsSecretNameField      = ".spec.credentialsSecretName" //nolint:gosec

	databaseClusterNameLabel        = "clusterName"
	monitoringConfigNameLabel       = "monitoringConfigName"
	backupStorageNameLabelTmpl      = "backupStorage-%s"
	backupStorageLabelValue         = "used"
	finalizerDeletePXCPodsInOrder   = "delete-pxc-pods-in-order"
	finalizerDeletePSMDBPodsInOrder = "delete-psmdb-pods-in-order"
	finalizerDeletePXCPVC           = "delete-pxc-pvc"
	finalizerDeletePSMDBPVC         = "delete-psmdb-pvc"
	finalizerDeletePGPVC            = "percona.com/delete-pvc"
	finalizerDeletePXCSSL           = "delete-ssl"
	finalizerDeletePGSSL            = "percona.com/delete-ssl"
	pgBackRestPathTmpl              = "%s-path"
	pgBackRestRetentionTmpl         = "%s-retention-full"
	pgBackRestStorageVerifyTmpl     = "%s-storage-verify-tls"

	everestSecretsPrefix = "everest-secrets-"
)

var (
	operatorDeployment = map[everestv1alpha1.EngineType]string{
		everestv1alpha1.DatabaseEnginePXC:        pxcDeploymentName,
		everestv1alpha1.DatabaseEnginePSMDB:      psmdbDeploymentName,
		everestv1alpha1.DatabaseEnginePostgresql: pgDeploymentName,
	}

	maxUnavailable       = intstr.FromInt(1)
	exposeAnnotationsMap = map[ClusterType]map[string]string{
		ClusterTypeEKS: {
			"service.beta.kubernetes.io/aws-load-balancer-type": "nlb",
		},
	}
	memorySmallSize  = resource.MustParse("2G")
	memoryMediumSize = resource.MustParse("8G")
	memoryLargeSize  = resource.MustParse("32G")

	errPSMDBOneStorageRestriction = errors.New("using more than one storage is not allowed for psmdb clusters")
)

// DatabaseClusterReconciler reconciles a DatabaseCluster object.
type DatabaseClusterReconciler struct {
	client.Client
	Scheme              *runtime.Scheme
	systemNamespace     string
	monitoringNamespace string
}

//+kubebuilder:rbac:groups=everest.percona.com,resources=databaseclusters,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=everest.percona.com,resources=databaseclusters/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=everest.percona.com,resources=databaseclusters/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch
//+kubebuilder:rbac:groups=storage.k8s.io,resources=storageclasses,verbs=get;list;watch
//+kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch
//+kubebuilder:rbac:groups=pxc.percona.com,resources=perconaxtradbclusters,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=psmdb.percona.com,resources=perconaservermongodbs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=pgv2.percona.com,resources=perconapgclusters,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=everest.percona.com,resources=monitoringconfigs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=everest.percona.com,resources=backupstorages,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.1/pkg/reconcile
func (r *DatabaseClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling", "request", req)

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
	logger.Info("Reconciled", "request", req)

	if err := r.reconcileLabels(ctx, database); err != nil {
		return reconcile.Result{}, err
	}

	_, ok := database.ObjectMeta.Annotations[restartAnnotationKey]
	if ok && !database.Spec.Paused {
		logger.Info("Pausing database cluster")
		database.Spec.Paused = true
		if err := r.Update(ctx, database); err != nil {
			return reconcile.Result{}, err
		}
	}
	if ok && database.Status.Status == everestv1alpha1.AppStatePaused && database.Status.Ready == 0 {
		logger.Info("Unpausing database cluster")
		database.Spec.Paused = false
		delete(database.ObjectMeta.Annotations, restartAnnotationKey)
		if err := r.Update(ctx, database); err != nil {
			return reconcile.Result{}, err
		}
	}
	if database.Spec.Engine.UserSecretsName == "" {
		database.Spec.Engine.UserSecretsName = everestSecretsPrefix + database.Name
	}
	if database.Spec.Engine.Replicas == 0 {
		database.Spec.Engine.Replicas = 3
	}
	if database.Spec.Engine.Replicas == 1 && !database.Spec.AllowUnsafeConfiguration {
		database.Spec.AllowUnsafeConfiguration = true
		if err := r.Update(ctx, database); err != nil {
			return reconcile.Result{}, err
		}
	}

	if database.Spec.DataSource != nil && database.Spec.DataSource.DBClusterBackupName != "" {
		// We don't handle database.Spec.DataSource.BackupSource in operator
		if err := r.copyCredentialsFromDBBackup(ctx, database.Spec.DataSource.DBClusterBackupName, database); err != nil {
			return reconcile.Result{}, err
		}
	}

	if database.Spec.Engine.Type == everestv1alpha1.DatabaseEnginePXC {
		err := r.reconcilePXC(ctx, req, database)
		return reconcile.Result{}, err
	}
	if database.Spec.Engine.Type == everestv1alpha1.DatabaseEnginePSMDB {
		err := r.reconcilePSMDB(ctx, req, database)
		if err != nil {
			logger.Error(err, "unable to reconcile psmdb")
		}
		return reconcile.Result{}, err
	}
	if database.Spec.Engine.Type == everestv1alpha1.DatabaseEnginePostgresql {
		err := r.reconcilePG(ctx, req, database)
		return reconcile.Result{}, err
	}
	return reconcile.Result{}, nil
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
		return errors.Join(err, errors.New("could not get DB backup to copy credentials from old DB cluster"))
	}

	newSecretName := everestSecretsPrefix + db.Name
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

	prevSecretName := everestSecretsPrefix + dbb.Spec.DBClusterName
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
	if err := r.createOrUpdate(ctx, secret, false); err != nil {
		return errors.Join(err, errors.New("could not create new secret to copy credentials from old DB cluster"))
	}

	logger.Info(fmt.Sprintf("Copied secret %s to %s", prevSecretName, newSecretName))

	return nil
}

func (r *DatabaseClusterReconciler) getClusterType(ctx context.Context) (ClusterType, error) {
	clusterType := ClusterTypeMinikube
	unstructuredResource := &unstructured.Unstructured{}
	unstructuredResource.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "storage.k8s.io",
		Kind:    "StorageClass",
		Version: "v1",
	})
	storageList := &storagev1.StorageClassList{}

	err := r.List(ctx, unstructuredResource)
	if err != nil {
		return clusterType, err
	}
	err = runtime.DefaultUnstructuredConverter.
		FromUnstructured(unstructuredResource.Object, storageList)
	if err != nil {
		return clusterType, err
	}
	for _, storage := range storageList.Items {
		if strings.Contains(storage.Provisioner, "aws") {
			clusterType = ClusterTypeEKS
		}
	}
	return clusterType, nil
}

func (r *DatabaseClusterReconciler) reconcileDBRestoreFromDataSource(ctx context.Context, database *everestv1alpha1.DatabaseCluster) error {
	if (database.Spec.DataSource.DBClusterBackupName == "" && database.Spec.DataSource.BackupSource == nil) ||
		(database.Spec.DataSource.DBClusterBackupName != "" && database.Spec.DataSource.BackupSource != nil) {
		return errors.New("either DBClusterBackupName or BackupSource must be specified in the DataSource field")
	}

	dbRestore := &everestv1alpha1.DatabaseClusterRestore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      database.Name,
			Namespace: database.Namespace,
		},
	}
	if err := controllerutil.SetControllerReference(database, dbRestore, r.Client.Scheme()); err != nil {
		return err
	}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, dbRestore, func() error {
		dbRestore.Spec.DBClusterName = database.Name
		dbRestore.Spec.DataSource = r.getRestoreDataSource(ctx, database)
		return nil
	})

	return err
}

func (r *DatabaseClusterReconciler) getRestoreDataSource(ctx context.Context, database *everestv1alpha1.DatabaseCluster) everestv1alpha1.DataSource {
	if database.Spec.Engine.Type != everestv1alpha1.DatabaseEnginePSMDB || database.Spec.DataSource.DBClusterBackupName == "" {
		return *database.Spec.DataSource
	}

	// Use the full backup source instead of just the backupName to be able to
	// figure out the source dbc's prefix in the storage
	backupName := database.Spec.DataSource.DBClusterBackupName

	backup := &psmdbv1.PerconaServerMongoDBBackup{}
	err := r.Get(ctx, types.NamespacedName{Name: backupName, Namespace: database.Namespace}, backup)
	if err != nil {
		return everestv1alpha1.DataSource{}
	}

	return everestv1alpha1.DataSource{
		BackupSource: &everestv1alpha1.BackupSource{
			Path:              backup.Status.Destination,
			BackupStorageName: backup.Spec.StorageName,
		},
		PITR: database.Spec.DataSource.PITR,
	}
}

//nolint:gocognit,gocyclo
func (r *DatabaseClusterReconciler) genPSMDBBackupSpec( //nolint:cyclop,maintidx
	ctx context.Context,
	database *everestv1alpha1.DatabaseCluster,
	engine *everestv1alpha1.DatabaseEngine,
) (psmdbv1.BackupSpec, error) {
	emptySpec := psmdbv1.BackupSpec{Enabled: false}
	bestBackupVersion := engine.BestBackupVersion(database.Spec.Engine.Version)
	backupVersion, ok := engine.Status.AvailableVersions.Backup[bestBackupVersion]
	if !ok {
		return emptySpec, fmt.Errorf("backup version %s not available", bestBackupVersion)
	}

	psmdbBackupSpec := psmdbv1.BackupSpec{
		Enabled: true,
		Image:   backupVersion.ImagePath,
		PITR: psmdbv1.PITRSpec{
			Enabled: database.Spec.Backup.PITR.Enabled,
		},

		// XXX: Remove this once templates will be available
		Resources: corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("1G"),
				corev1.ResourceCPU:    resource.MustParse("300m"),
			},
		},
	}

	storages := make(map[string]psmdbv1.BackupStorageSpec)

	// List DatabaseClusterBackup objects for this database
	backupList := &everestv1alpha1.DatabaseClusterBackupList{}
	listOps := &client.ListOptions{
		FieldSelector: fields.OneTermEqualSelector(dbClusterBackupDBClusterNameField, database.Name),
		Namespace:     database.Namespace,
	}
	err := r.List(ctx, backupList, listOps)
	if err != nil {
		return emptySpec, errors.Join(err, errors.New("failed to list DatabaseClusterBackup objects"))
	}

	// Add the storages used by the DatabaseClusterBackup objects
	for _, backup := range backupList.Items {
		// Check if we already fetched that backup storage
		if _, ok := storages[backup.Spec.BackupStorageName]; ok {
			continue
		}

		backupStorage := &everestv1alpha1.BackupStorage{}
		err := r.Get(ctx, types.NamespacedName{
			Name:      backup.Spec.BackupStorageName,
			Namespace: r.systemNamespace,
		}, backupStorage)
		if err != nil {
			return emptySpec, errors.Join(err, fmt.Errorf("failed to get backup storage %s", backup.Spec.BackupStorageName))
		}
		verifyTLS := pointer.Get(backupStorage.Spec.VerifyTLS)
		if database.Namespace != r.systemNamespace {
			if err := r.reconcileBackupStorageSecret(ctx, backupStorage, database); err != nil {
				return emptySpec, err
			}
		}

		switch backupStorage.Spec.Type {
		case everestv1alpha1.BackupStorageTypeS3:
			storages[backup.Spec.BackupStorageName] = psmdbv1.BackupStorageSpec{
				Type: psmdbv1.BackupStorageS3,
				S3: psmdbv1.BackupStorageS3Spec{
					Bucket:                backupStorage.Spec.Bucket,
					CredentialsSecret:     backupStorage.Spec.CredentialsSecretName,
					Region:                backupStorage.Spec.Region,
					EndpointURL:           backupStorage.Spec.EndpointURL,
					Prefix:                backupStoragePrefix(database),
					InsecureSkipTLSVerify: !verifyTLS,
				},
			}
		case everestv1alpha1.BackupStorageTypeAzure:
			storages[backup.Spec.BackupStorageName] = psmdbv1.BackupStorageSpec{
				Type: psmdbv1.BackupStorageAzure,
				Azure: psmdbv1.BackupStorageAzureSpec{
					Container:         backupStorage.Spec.Bucket,
					Prefix:            backupStoragePrefix(database),
					CredentialsSecret: backupStorage.Spec.CredentialsSecretName,
				},
			}
		default:
			return emptySpec, fmt.Errorf("unsupported backup storage type %s for %s", backupStorage.Spec.Type, backupStorage.Name)
		}
	}

	// List DatabaseClusterRestore objects for this database
	restoreList := &everestv1alpha1.DatabaseClusterRestoreList{}
	listOps = &client.ListOptions{
		FieldSelector: fields.OneTermEqualSelector(dbClusterRestoreDBClusterNameField, database.Name),
		Namespace:     database.Namespace,
	}
	err = r.List(ctx, restoreList, listOps)
	if err != nil {
		return emptySpec, errors.Join(err, errors.New("failed to list DatabaseClusterRestore objects"))
	}

	// Add used restore backup storages to the list
	for _, restore := range restoreList.Items {
		// If the restore has already completed, skip it.
		if restore.IsComplete(database.Spec.Engine.Type) {
			continue
		}
		// Restores using the BackupSource field instead of the
		// DBClusterBackupName don't need to have the storage defined, skip
		// them.
		if restore.Spec.DataSource.DBClusterBackupName == "" {
			continue
		}

		backup := &everestv1alpha1.DatabaseClusterBackup{}
		err := r.Get(ctx, types.NamespacedName{Name: restore.Spec.DataSource.DBClusterBackupName, Namespace: restore.Namespace}, backup)
		if err != nil {
			return emptySpec, errors.Join(err, errors.New("failed to get DatabaseClusterBackup"))
		}

		// Check if we already fetched that backup storage
		if _, ok := storages[backup.Spec.BackupStorageName]; ok {
			continue
		}

		backupStorage := &everestv1alpha1.BackupStorage{}
		err = r.Get(ctx, types.NamespacedName{Name: backup.Spec.BackupStorageName, Namespace: r.systemNamespace}, backupStorage)
		if err != nil {
			return emptySpec, errors.Join(err, fmt.Errorf("failed to get backup storage %s", backup.Spec.BackupStorageName))
		}
		verifyTLS := pointer.Get(backupStorage.Spec.VerifyTLS)
		if database.Namespace != r.systemNamespace {
			if err := r.reconcileBackupStorageSecret(ctx, backupStorage, database); err != nil {
				return emptySpec, err
			}
		}

		switch backupStorage.Spec.Type {
		case everestv1alpha1.BackupStorageTypeS3:
			storages[backup.Spec.BackupStorageName] = psmdbv1.BackupStorageSpec{
				Type: psmdbv1.BackupStorageS3,
				S3: psmdbv1.BackupStorageS3Spec{
					Bucket:                backupStorage.Spec.Bucket,
					CredentialsSecret:     backupStorage.Spec.CredentialsSecretName,
					Region:                backupStorage.Spec.Region,
					EndpointURL:           backupStorage.Spec.EndpointURL,
					Prefix:                backupStoragePrefix(database),
					InsecureSkipTLSVerify: !verifyTLS,
				},
			}
		case everestv1alpha1.BackupStorageTypeAzure:
			storages[backup.Spec.BackupStorageName] = psmdbv1.BackupStorageSpec{
				Type: psmdbv1.BackupStorageAzure,
				Azure: psmdbv1.BackupStorageAzureSpec{
					Container:         backupStorage.Spec.Bucket,
					Prefix:            backupStoragePrefix(database),
					CredentialsSecret: backupStorage.Spec.CredentialsSecretName,
				},
			}
		default:
			return emptySpec, fmt.Errorf("unsupported backup storage type %s for %s", backupStorage.Spec.Type, backupStorage.Name)
		}
	}

	// If scheduled backups are disabled, just return the storages used in
	// DatabaseClusterBackup objects
	if !database.Spec.Backup.Enabled {
		if len(storages) > 1 {
			return emptySpec, errPSMDBOneStorageRestriction
		}
		psmdbBackupSpec.Storages = storages
		return psmdbBackupSpec, nil
	}

	var tasks []psmdbv1.BackupTaskSpec //nolint:prealloc
	for _, schedule := range database.Spec.Backup.Schedules {
		if !schedule.Enabled {
			continue
		}

		backupStorage := &everestv1alpha1.BackupStorage{}
		err := r.Get(ctx, types.NamespacedName{Name: schedule.BackupStorageName, Namespace: r.systemNamespace}, backupStorage)
		if err != nil {
			return emptySpec, errors.Join(err, fmt.Errorf("failed to get backup storage %s", schedule.BackupStorageName))
		}
		if database.Namespace != r.systemNamespace {
			if err := r.reconcileBackupStorageSecret(ctx, backupStorage, database); err != nil {
				return emptySpec, err
			}
		}

		verifyTLS := pointer.Get(backupStorage.Spec.VerifyTLS)
		switch backupStorage.Spec.Type {
		case everestv1alpha1.BackupStorageTypeS3:
			storages[schedule.BackupStorageName] = psmdbv1.BackupStorageSpec{
				Type: psmdbv1.BackupStorageS3,
				S3: psmdbv1.BackupStorageS3Spec{
					Bucket:                backupStorage.Spec.Bucket,
					CredentialsSecret:     backupStorage.Spec.CredentialsSecretName,
					Region:                backupStorage.Spec.Region,
					EndpointURL:           backupStorage.Spec.EndpointURL,
					Prefix:                backupStoragePrefix(database),
					InsecureSkipTLSVerify: !verifyTLS,
				},
			}
		case everestv1alpha1.BackupStorageTypeAzure:
			storages[schedule.BackupStorageName] = psmdbv1.BackupStorageSpec{
				Type: psmdbv1.BackupStorageAzure,
				Azure: psmdbv1.BackupStorageAzureSpec{
					Container:         backupStorage.Spec.Bucket,
					Prefix:            backupStoragePrefix(database),
					CredentialsSecret: backupStorage.Spec.CredentialsSecretName,
				},
			}
		default:
			return emptySpec, fmt.Errorf("unsupported backup storage type %s for %s", backupStorage.Spec.Type, backupStorage.Name)
		}

		tasks = append(tasks, psmdbv1.BackupTaskSpec{
			Name:        schedule.Name,
			Enabled:     true,
			Schedule:    schedule.Schedule,
			Keep:        int(schedule.RetentionCopies),
			StorageName: schedule.BackupStorageName,
		})
	}

	if database.Spec.Backup.PITR.Enabled {
		interval := database.Spec.Backup.PITR.UploadIntervalSec
		if interval != nil {
			// the integer amount of minutes. default 10
			psmdbBackupSpec.PITR.OplogSpanMin = numstr.NumberString(strconv.Itoa(*interval / 60))
		}
	}

	if len(storages) > 1 {
		return emptySpec, errPSMDBOneStorageRestriction
	}

	psmdbBackupSpec.Storages = storages
	psmdbBackupSpec.Tasks = tasks

	return psmdbBackupSpec, nil
}

func (r *DatabaseClusterReconciler) reconcilePSMDB(ctx context.Context, req ctrl.Request, database *everestv1alpha1.DatabaseCluster) error { //nolint:gocognit,maintidx,gocyclo,lll,cyclop
	version, err := r.getOperatorVersion(ctx, types.NamespacedName{
		Namespace: req.NamespacedName.Namespace,
		Name:      psmdbDeploymentName,
	})
	if err != nil {
		return err
	}

	psmdb := &psmdbv1.PerconaServerMongoDB{}
	err = r.Get(ctx, types.NamespacedName{Name: database.Name, Namespace: database.Namespace}, psmdb)
	if err != nil && !k8serrors.IsNotFound(err) {
		return errors.Join(err, fmt.Errorf("failed to get psmdb object %s", database.Name))
	}
	psmdb.Name = database.Name
	psmdb.Namespace = database.Namespace
	psmdb.Annotations = database.Annotations
	if len(database.Finalizers) != 0 {
		psmdb.Finalizers = database.Finalizers
		database.Finalizers = []string{}
	}
	if err := r.Update(ctx, database); err != nil {
		return err
	}
	engine := &everestv1alpha1.DatabaseEngine{}
	err = r.Get(ctx, types.NamespacedName{Namespace: database.Namespace, Name: operatorDeployment[database.Spec.Engine.Type]}, engine)
	if err != nil {
		return errors.Join(err, fmt.Errorf("failed to get database engine %s", operatorDeployment[database.Spec.Engine.Type]))
	}

	var activeStorage string
	if err := controllerutil.SetControllerReference(database, psmdb, r.Client.Scheme()); err != nil {
		return err
	}
	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, psmdb, func() error {
		if !controllerutil.ContainsFinalizer(psmdb, finalizerDeletePSMDBPodsInOrder) {
			controllerutil.AddFinalizer(psmdb, finalizerDeletePSMDBPodsInOrder)
		}
		if !controllerutil.ContainsFinalizer(psmdb, finalizerDeletePSMDBPVC) {
			controllerutil.AddFinalizer(psmdb, finalizerDeletePSMDBPVC)
		}
		psmdb.TypeMeta = metav1.TypeMeta{
			APIVersion: version.ToAPIVersion(psmdbAPIGroup),
			Kind:       PerconaServerMongoDBKind,
		}
		crVersion := version.ToCRVersion()
		if psmdb.Spec.CRVersion != "" {
			crVersion = psmdb.Spec.CRVersion
		}

		clusterType, err := r.getClusterType(ctx)
		if err != nil {
			return err
		}

		psmdb.Spec = *r.defaultPSMDBSpec()
		if clusterType == ClusterTypeEKS {
			affinity := &psmdbv1.PodAffinity{
				TopologyKey: pointer.ToString("kubernetes.io/hostname"),
			}
			psmdb.Spec.Replsets[0].MultiAZ.Affinity = affinity
		}

		dbTemplateKind, hasTemplateKind := database.ObjectMeta.Annotations[dbTemplateKindAnnotationKey]
		dbTemplateName, hasTemplateName := database.ObjectMeta.Annotations[dbTemplateNameAnnotationKey]
		if hasTemplateKind && !hasTemplateName {
			return fmt.Errorf("missing %s annotation", dbTemplateNameAnnotationKey)
		}
		if !hasTemplateKind && hasTemplateName {
			return fmt.Errorf("missing %s annotation", dbTemplateKindAnnotationKey)
		}
		if hasTemplateKind && hasTemplateName {
			err := r.applyTemplate(ctx, psmdb, dbTemplateKind, types.NamespacedName{
				Namespace: req.NamespacedName.Namespace,
				Name:      dbTemplateName,
			})
			if err != nil {
				return err
			}
		}

		psmdb.Spec.CRVersion = crVersion
		psmdb.Spec.UnsafeConf = database.Spec.AllowUnsafeConfiguration
		psmdb.Spec.Pause = database.Spec.Paused

		if database.Spec.Engine.Version == "" {
			database.Spec.Engine.Version = engine.BestEngineVersion()
		}
		engineVersion, ok := engine.Status.AvailableVersions.Engine[database.Spec.Engine.Version]
		if !ok {
			return fmt.Errorf("engine version %s not available", database.Spec.Engine.Version)
		}

		psmdb.Spec.Image = engineVersion.ImagePath

		psmdb.Spec.Secrets = &psmdbv1.SecretsSpec{
			Users:         database.Spec.Engine.UserSecretsName,
			EncryptionKey: database.Name + "-mongodb-encryption-key",
		}

		if database.Spec.Engine.Config != "" {
			psmdb.Spec.Replsets[0].Configuration = psmdbv1.MongoConfiguration(database.Spec.Engine.Config)
		}
		if psmdb.Spec.Replsets[0].Configuration == "" {
			// Config missing from the DatabaseCluster CR and the template (if any), apply the default one
			psmdb.Spec.Replsets[0].Configuration = psmdbv1.MongoConfiguration(psmdbDefaultConfigurationTemplate)
		}

		psmdb.Spec.Replsets[0].Size = database.Spec.Engine.Replicas
		psmdb.Spec.Replsets[0].VolumeSpec = &psmdbv1.VolumeSpec{
			PersistentVolumeClaim: psmdbv1.PVCSpec{
				PersistentVolumeClaimSpec: &corev1.PersistentVolumeClaimSpec{
					StorageClassName: database.Spec.Engine.Storage.Class,
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: database.Spec.Engine.Storage.Size,
						},
					},
				},
			},
		}
		if !database.Spec.Engine.Resources.CPU.IsZero() {
			psmdb.Spec.Replsets[0].MultiAZ.Resources.Limits[corev1.ResourceCPU] = database.Spec.Engine.Resources.CPU
		}
		if !database.Spec.Engine.Resources.Memory.IsZero() {
			psmdb.Spec.Replsets[0].MultiAZ.Resources.Limits[corev1.ResourceMemory] = database.Spec.Engine.Resources.Memory
		}
		switch database.Spec.Proxy.Expose.Type {
		case everestv1alpha1.ExposeTypeInternal:
			psmdb.Spec.Replsets[0].Expose.Enabled = false
			psmdb.Spec.Replsets[0].Expose.ExposeType = corev1.ServiceTypeClusterIP
		case everestv1alpha1.ExposeTypeExternal:
			psmdb.Spec.Replsets[0].Expose.Enabled = true
			psmdb.Spec.Replsets[0].Expose.ExposeType = corev1.ServiceTypeLoadBalancer
			psmdb.Spec.Replsets[0].Expose.LoadBalancerSourceRanges = database.Spec.Proxy.Expose.IPSourceRangesStringArray()
		default:
			return fmt.Errorf("invalid expose type %s", database.Spec.Proxy.Expose.Type)
		}

		monitoring := &everestv1alpha1.MonitoringConfig{}
		if database.Spec.Monitoring != nil && database.Spec.Monitoring.MonitoringConfigName != "" {
			err := r.Get(ctx, types.NamespacedName{
				Namespace: r.monitoringNamespace,
				Name:      database.Spec.Monitoring.MonitoringConfigName,
			}, monitoring)
			if err != nil {
				return err
			}
			if !monitoring.IsNamespaceAllowed(database.Namespace) {
				return fmt.Errorf("%s namespace is not allowed to use for %s monitoring config", database.Namespace, monitoring.Name)
			}
		}

		if monitoring.Spec.Type == everestv1alpha1.PMMMonitoringType { //nolint:dupl
			image := defaultPMMClientImage
			if monitoring.Spec.PMM.Image != "" {
				image = monitoring.Spec.PMM.Image
			}

			psmdb.Spec.PMM.Enabled = true
			pmmURL, err := url.Parse(monitoring.Spec.PMM.URL)
			if err != nil {
				return errors.Join(err, errors.New("invalid monitoring URL"))
			}
			psmdb.Spec.PMM.ServerHost = pmmURL.Hostname()
			psmdb.Spec.PMM.Image = image
			psmdb.Spec.PMM.Resources = database.Spec.Monitoring.Resources

			apiKey, err := r.getSecretFromMonitoringConfig(ctx, monitoring)
			if err != nil {
				return err
			}

			err = r.createOrUpdateSecretData(ctx, database, psmdb.Spec.Secrets.Users,
				map[string][]byte{
					"PMM_SERVER_API_KEY": []byte(apiKey),
				},
			)
			if err != nil {
				return err
			}
		}

		psmdb.Spec.Backup, err = r.genPSMDBBackupSpec(ctx, database, engine)
		if err != nil {
			return err
		}
		activeStorage = getActiveStorage(psmdb)
		return nil
	})
	if err != nil {
		return err
	}

	if database.Spec.DataSource != nil && database.Status.Status == everestv1alpha1.AppStateReady {
		err = r.reconcileDBRestoreFromDataSource(ctx, database)
		if err != nil {
			return err
		}
	}

	database.Status.Status = everestv1alpha1.AppState(psmdb.Status.State)
	// If a restore is running for this database, set the database status to
	// restoring
	restoreList := &everestv1alpha1.DatabaseClusterRestoreList{}
	listOps := &client.ListOptions{
		FieldSelector: fields.OneTermEqualSelector(dbClusterRestoreDBClusterNameField, database.Name),
		Namespace:     database.Namespace,
	}
	err = r.List(ctx, restoreList, listOps)
	if err != nil {
		return errors.Join(err, errors.New("failed to list DatabaseClusterRestore objects"))
	}
	for _, restore := range restoreList.Items {
		if !restore.IsComplete(database.Spec.Engine.Type) {
			database.Status.Status = everestv1alpha1.AppStateRestoring
			break
		}
	}

	database.Status.Hostname = psmdb.Status.Host
	database.Status.Ready = psmdb.Status.Ready
	database.Status.Size = psmdb.Status.Size
	message := psmdb.Status.Message
	conditions := psmdb.Status.Conditions
	if message == "" && len(conditions) != 0 {
		message = conditions[len(conditions)-1].Message
	}
	database.Status.Message = message
	database.Status.Port = 27017
	database.Status.ActiveStorage = activeStorage

	return r.Status().Update(ctx, database)
}

// getSecretFromMonitoringConfig retrieves the credentials secret from the provided MonitoringConfig.
func (r *DatabaseClusterReconciler) getSecretFromMonitoringConfig(
	ctx context.Context, monitoring *everestv1alpha1.MonitoringConfig,
) (string, error) {
	var secret *corev1.Secret
	secretData := ""

	if monitoring.Spec.CredentialsSecretName != "" {
		secret = &corev1.Secret{}
		err := r.Get(ctx, types.NamespacedName{
			Name:      monitoring.Spec.CredentialsSecretName,
			Namespace: r.monitoringNamespace,
		}, secret)
		if err != nil {
			return "", err
		}

		if key, ok := secret.Data[everestv1alpha1.MonitoringConfigCredentialsSecretAPIKeyKey]; ok {
			secretData = string(key)
		}
	}

	return secretData, nil
}

// updateSecretData updates the data of a secret.
// It only changes the values of the keys specified in the data map.
// All other keys are left untouched, so it's not possible to delete a key.
func (r *DatabaseClusterReconciler) updateSecretData(
	ctx context.Context, database *everestv1alpha1.DatabaseCluster,
	secretName string, data map[string][]byte,
) error {
	secret := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      secretName,
		Namespace: database.Namespace,
	}, secret)
	if err != nil {
		return err
	}

	var needsUpdate bool
	for k, v := range data {
		oldValue, ok := secret.Data[k]
		if !ok || !bytes.Equal(oldValue, v) {
			secret.Data[k] = v
			needsUpdate = true
		}
	}
	if !needsUpdate {
		return nil
	}

	err = controllerutil.SetControllerReference(database, secret, r.Scheme)
	if err != nil {
		return err
	}
	return r.Update(ctx, secret)
}

// createOrUpdateSecretData creates or updates the data of a secret.
// When updating, it only changes the values of the keys specified in the data
// map.
// All other keys are left untouched, so it's not possible to delete a key.
func (r *DatabaseClusterReconciler) createOrUpdateSecretData(
	ctx context.Context, database *everestv1alpha1.DatabaseCluster,
	secretName string, data map[string][]byte,
) error {
	err := r.updateSecretData(ctx, database, secretName, data)
	if err == nil {
		return nil
	}

	if !k8serrors.IsNotFound(err) {
		return err
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: database.Namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: data,
	}
	err = controllerutil.SetControllerReference(database, secret, r.Scheme)
	if err != nil {
		return err
	}
	// If the secret does not exist, create it
	return r.Create(ctx, secret)
}

func (r *DatabaseClusterReconciler) genPXCHAProxySpec(
	ctx context.Context,
	database *everestv1alpha1.DatabaseCluster,
	engine *everestv1alpha1.DatabaseEngine,
	clusterType ClusterType,
) (*pxcv1.HAProxySpec, error) {
	haProxy := r.defaultPXCSpec().HAProxy
	// Set configuration based on database size.
	if database.Spec.Engine.Resources.Memory.Cmp(memoryLargeSize) == 0 {
		haProxy.PodSpec.Resources = haProxyResourceRequirementsLarge
	}
	if database.Spec.Engine.Resources.Memory.Cmp(memoryMediumSize) == 0 {
		haProxy.PodSpec.Resources = haProxyResourceRequirementsMedium
	}
	if database.Spec.Engine.Resources.Memory.Cmp(memorySmallSize) == 0 {
		haProxy.PodSpec.Resources = haProxyResourceRequirementsSmall
	}
	haProxy.PodSpec.Enabled = true

	if database.Spec.Proxy.Replicas == nil {
		// By default we set the same number of replicas as the engine
		haProxy.PodSpec.Size = database.Spec.Engine.Replicas
	} else {
		haProxy.PodSpec.Size = *database.Spec.Proxy.Replicas
	}

	switch database.Spec.Proxy.Expose.Type {
	case everestv1alpha1.ExposeTypeInternal:
		haProxy.PodSpec.ServiceType = corev1.ServiceTypeClusterIP
		haProxy.PodSpec.ReplicasServiceType = corev1.ServiceTypeClusterIP
	case everestv1alpha1.ExposeTypeExternal:
		haProxy.PodSpec.ServiceType = corev1.ServiceTypeLoadBalancer
		haProxy.PodSpec.ReplicasServiceType = corev1.ServiceTypeLoadBalancer
		haProxy.PodSpec.LoadBalancerSourceRanges = database.Spec.Proxy.Expose.IPSourceRangesStringArray()
		annotations, ok := exposeAnnotationsMap[clusterType]
		if ok {
			haProxy.PodSpec.ServiceAnnotations = annotations
			haProxy.PodSpec.ReplicasServiceAnnotations = annotations
		}
	default:
		return nil, fmt.Errorf("invalid expose type %s", database.Spec.Proxy.Expose.Type)
	}

	haProxy.PodSpec.Configuration = database.Spec.Proxy.Config
	if haProxy.PodSpec.Configuration == "" {
		haProxy.PodSpec.Configuration = haProxyConfigDefault
	}

	// Ensure there is a env vars secret for HAProxy
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pxcHAProxyEnvSecretName,
			Namespace: database.GetNamespace(),
		},
	}
	if err := controllerutil.SetControllerReference(database, secret, r.Client.Scheme()); err != nil {
		return nil, fmt.Errorf("failed to set controller reference for secret %s", pxcHAProxyEnvSecretName)
	}
	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, secret, func() error {
		secret.Data = haProxyEnvVars
		return nil
	}); err != nil {
		return nil, fmt.Errorf("failed to create or update secret %w", err)
	}
	haProxy.PodSpec.EnvVarsSecretName = pxcHAProxyEnvSecretName

	haProxyAvailVersions, ok := engine.Status.AvailableVersions.Proxy[everestv1alpha1.ProxyTypeHAProxy]
	if !ok {
		return nil, errors.New("haproxy version not available")
	}

	bestHAProxyVersion := haProxyAvailVersions.BestVersion()
	haProxyVersion, ok := haProxyAvailVersions[bestHAProxyVersion]
	if !ok {
		return nil, fmt.Errorf("haproxy version %s not available", bestHAProxyVersion)
	}

	haProxy.PodSpec.Image = haProxyVersion.ImagePath

	if !database.Spec.Proxy.Resources.CPU.IsZero() {
		haProxy.PodSpec.Resources.Limits[corev1.ResourceCPU] = database.Spec.Proxy.Resources.CPU
	}
	if !database.Spec.Proxy.Resources.Memory.IsZero() {
		haProxy.PodSpec.Resources.Limits[corev1.ResourceMemory] = database.Spec.Proxy.Resources.Memory
	}

	return haProxy, nil
}

func (r *DatabaseClusterReconciler) genPXCProxySQLSpec(database *everestv1alpha1.DatabaseCluster, engine *everestv1alpha1.DatabaseEngine) (*pxcv1.PodSpec, error) {
	proxySQL := r.defaultPXCSpec().ProxySQL

	proxySQL.Enabled = true

	if database.Spec.Proxy.Replicas == nil {
		// By default we set the same number of replicas as the engine
		proxySQL.Size = database.Spec.Engine.Replicas
	} else {
		proxySQL.Size = *database.Spec.Proxy.Replicas
	}

	switch database.Spec.Proxy.Expose.Type {
	case everestv1alpha1.ExposeTypeInternal:
		proxySQL.ServiceType = corev1.ServiceTypeClusterIP
		proxySQL.ReplicasServiceType = corev1.ServiceTypeClusterIP
	case everestv1alpha1.ExposeTypeExternal:
		proxySQL.ServiceType = corev1.ServiceTypeLoadBalancer
		proxySQL.ReplicasServiceType = corev1.ServiceTypeLoadBalancer
		proxySQL.LoadBalancerSourceRanges = database.Spec.Proxy.Expose.IPSourceRangesStringArray()
	default:
		return nil, fmt.Errorf("invalid expose type %s", database.Spec.Proxy.Expose.Type)
	}

	proxySQL.Configuration = database.Spec.Proxy.Config

	proxySQLAvailVersions, ok := engine.Status.AvailableVersions.Proxy[everestv1alpha1.ProxyTypeProxySQL]
	if !ok {
		return nil, errors.New("proxysql version not available")
	}

	bestProxySQLVersion := proxySQLAvailVersions.BestVersion()
	proxySQLVersion, ok := proxySQLAvailVersions[bestProxySQLVersion]
	if !ok {
		return nil, fmt.Errorf("proxysql version %s not available", bestProxySQLVersion)
	}

	proxySQL.Image = proxySQLVersion.ImagePath

	if !database.Spec.Proxy.Resources.CPU.IsZero() {
		proxySQL.Resources.Limits[corev1.ResourceCPU] = database.Spec.Proxy.Resources.CPU
	}
	if !database.Spec.Proxy.Resources.Memory.IsZero() {
		proxySQL.Resources.Limits[corev1.ResourceMemory] = database.Spec.Proxy.Resources.Memory
	}

	return proxySQL, nil
}

func (r *DatabaseClusterReconciler) genPXCBackupSpec( //nolint:gocognit
	ctx context.Context,
	database *everestv1alpha1.DatabaseCluster,
	engine *everestv1alpha1.DatabaseEngine,
) (*pxcv1.PXCScheduledBackup, error) {
	bestBackupVersion := engine.BestBackupVersion(database.Spec.Engine.Version)
	backupVersion, ok := engine.Status.AvailableVersions.Backup[bestBackupVersion]
	if !ok {
		return nil, fmt.Errorf("backup version %s not available", bestBackupVersion)
	}

	pxcBackupSpec := &pxcv1.PXCScheduledBackup{
		Image: backupVersion.ImagePath,
		PITR: pxcv1.PITRSpec{
			Enabled: database.Spec.Backup.PITR.Enabled,
		},
	}

	storages := make(map[string]*pxcv1.BackupStorageSpec)

	// List DatabaseClusterBackup objects for this database
	backupList := &everestv1alpha1.DatabaseClusterBackupList{}
	listOps := &client.ListOptions{
		FieldSelector: fields.OneTermEqualSelector(dbClusterBackupDBClusterNameField, database.Name),
		Namespace:     database.Namespace,
	}
	err := r.List(ctx, backupList, listOps)
	if err != nil {
		return nil, errors.Join(err, errors.New("failed to list DatabaseClusterBackup objects"))
	}

	// Add the storages used by the DatabaseClusterBackup objects
	for _, backup := range backupList.Items {
		if _, ok := storages[backup.Spec.BackupStorageName]; ok {
			continue
		}

		spec, backupStorage, err := r.genPXCStorageSpec(ctx, backup.Spec.BackupStorageName, r.systemNamespace)
		if err != nil {
			return nil, errors.Join(err, fmt.Errorf("failed to get backup storage for backup %s", backup.Name))
		}
		if database.Namespace != r.systemNamespace {
			if err := r.reconcileBackupStorageSecret(ctx, backupStorage, database); err != nil {
				return nil, err
			}
		}

		storages[backup.Spec.BackupStorageName] = spec

		switch backupStorage.Spec.Type {
		case everestv1alpha1.BackupStorageTypeS3:
			storages[backup.Spec.BackupStorageName].S3 = &pxcv1.BackupStorageS3Spec{
				Bucket: fmt.Sprintf(
					"%s/%s",
					backupStorage.Spec.Bucket,
					backupStoragePrefix(database),
				),
				CredentialsSecret: backupStorage.Spec.CredentialsSecretName,
				Region:            backupStorage.Spec.Region,
				EndpointURL:       backupStorage.Spec.EndpointURL,
			}
		case everestv1alpha1.BackupStorageTypeAzure:
			storages[backup.Spec.BackupStorageName].Azure = &pxcv1.BackupStorageAzureSpec{
				ContainerPath: fmt.Sprintf(
					"%s/%s",
					backupStorage.Spec.Bucket,
					backupStoragePrefix(database),
				),
				CredentialsSecret: backupStorage.Spec.CredentialsSecretName,
			}
		default:
			return nil, fmt.Errorf("unsupported backup storage type %s for %s", backupStorage.Spec.Type, backupStorage.Name)
		}
	}

	if database.Spec.Backup.PITR.Enabled { //nolint:nestif
		storageName := *database.Spec.Backup.PITR.BackupStorageName
		spec, backupStorage, err := r.genPXCStorageSpec(ctx, storageName, r.systemNamespace)
		if err != nil {
			return nil, errors.Join(err, errors.New("failed to get pitr storage"))
		}
		pxcBackupSpec.PITR.StorageName = pitrStorageName(storageName)

		var timeBetweenUploads float64
		if database.Spec.Backup.PITR.UploadIntervalSec != nil {
			timeBetweenUploads = float64(*database.Spec.Backup.PITR.UploadIntervalSec)
		}
		pxcBackupSpec.PITR.TimeBetweenUploads = timeBetweenUploads

		switch backupStorage.Spec.Type {
		case everestv1alpha1.BackupStorageTypeS3:
			spec.S3 = &pxcv1.BackupStorageS3Spec{
				// use separate directory for binlogs
				Bucket:            pitrBucketName(database, backupStorage.Spec.Bucket),
				CredentialsSecret: backupStorage.Spec.CredentialsSecretName,
				Region:            backupStorage.Spec.Region,
				EndpointURL:       backupStorage.Spec.EndpointURL,
			}
		default:
			return nil, fmt.Errorf("BackupStorage of type %s is not supported. PITR only works for s3 compatible storages", backupStorage.Spec.Type)
		}

		if database.Namespace != r.systemNamespace {
			if err := r.reconcileBackupStorageSecret(ctx, backupStorage, database); err != nil {
				return nil, err
			}
		}

		// create a separate storage for pxc pitr as the docs recommend
		// https://docs.percona.com/percona-operator-for-mysql/pxc/backups-pitr.html
		storages[pitrStorageName(backupStorage.Name)] = spec
	}

	// If scheduled backups are disabled, just return the storages used in
	// DatabaseClusterBackup objects
	if !database.Spec.Backup.Enabled {
		pxcBackupSpec.Storages = storages
		return pxcBackupSpec, nil
	}

	var pxcSchedules []pxcv1.PXCScheduledBackupSchedule //nolint:prealloc
	for _, schedule := range database.Spec.Backup.Schedules {
		if !schedule.Enabled {
			continue
		}

		// Add the storages used by the schedule backups
		if _, ok := storages[schedule.BackupStorageName]; !ok {
			backupStorage := &everestv1alpha1.BackupStorage{}
			err := r.Get(ctx, types.NamespacedName{Name: schedule.BackupStorageName, Namespace: r.systemNamespace}, backupStorage)
			if err != nil {
				return nil, errors.Join(err, fmt.Errorf("failed to get backup storage %s", schedule.BackupStorageName))
			}
			if database.Namespace != r.systemNamespace {
				if err := r.reconcileBackupStorageSecret(ctx, backupStorage, database); err != nil {
					return nil, err
				}
			}

			storages[schedule.BackupStorageName] = &pxcv1.BackupStorageSpec{
				Type:      pxcv1.BackupStorageType(backupStorage.Spec.Type),
				VerifyTLS: backupStorage.Spec.VerifyTLS,
			}
			switch backupStorage.Spec.Type {
			case everestv1alpha1.BackupStorageTypeS3:
				storages[schedule.BackupStorageName].S3 = &pxcv1.BackupStorageS3Spec{
					Bucket: fmt.Sprintf(
						"%s/%s",
						backupStorage.Spec.Bucket,
						backupStoragePrefix(database),
					),
					CredentialsSecret: backupStorage.Spec.CredentialsSecretName,
					Region:            backupStorage.Spec.Region,
					EndpointURL:       backupStorage.Spec.EndpointURL,
				}
			case everestv1alpha1.BackupStorageTypeAzure:
				storages[schedule.BackupStorageName].Azure = &pxcv1.BackupStorageAzureSpec{
					ContainerPath: fmt.Sprintf(
						"%s/%s",
						backupStorage.Spec.Bucket,
						backupStoragePrefix(database),
					),
					CredentialsSecret: backupStorage.Spec.CredentialsSecretName,
				}
			default:
				return nil, fmt.Errorf("unsupported backup storage type %s for %s", backupStorage.Spec.Type, backupStorage.Name)
			}
		}

		pxcSchedules = append(pxcSchedules, pxcv1.PXCScheduledBackupSchedule{
			Name:        schedule.Name,
			Schedule:    schedule.Schedule,
			Keep:        int(schedule.RetentionCopies),
			StorageName: schedule.BackupStorageName,
		})
	}

	pxcBackupSpec.Storages = storages
	pxcBackupSpec.Schedule = pxcSchedules

	return pxcBackupSpec, nil
}

func pitrStorageName(storageName string) string {
	return storageName + "-pitr"
}

func pitrBucketName(db *everestv1alpha1.DatabaseCluster, bucket string) string {
	return fmt.Sprintf("%s/%s/%s", bucket, backupStoragePrefix(db), "pitr")
}

func (r *DatabaseClusterReconciler) genPXCStorageSpec(ctx context.Context, name, namespace string) (*pxcv1.BackupStorageSpec, *everestv1alpha1.BackupStorage, error) {
	backupStorage := &everestv1alpha1.BackupStorage{}
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, backupStorage)
	if err != nil {
		return nil, nil, errors.Join(err, fmt.Errorf("failed to get backup storage %s", name))
	}

	return &pxcv1.BackupStorageSpec{
		Type: pxcv1.BackupStorageType(backupStorage.Spec.Type),
		// XXX: Remove this once templates will be available
		Resources: corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("1G"),
				corev1.ResourceCPU:    resource.MustParse("600m"),
			},
		},
		VerifyTLS: backupStorage.Spec.VerifyTLS,
	}, backupStorage, nil
}

func (r *DatabaseClusterReconciler) reconcilePXC(ctx context.Context, req ctrl.Request, database *everestv1alpha1.DatabaseCluster) error { //nolint:lll,gocognit,gocyclo,cyclop,maintidx
	version, err := r.getOperatorVersion(ctx, types.NamespacedName{
		Namespace: req.NamespacedName.Namespace,
		Name:      pxcDeploymentName,
	})
	if err != nil {
		return err
	}

	pxc := &pxcv1.PerconaXtraDBCluster{}
	err = r.Get(ctx, types.NamespacedName{Name: database.Name, Namespace: database.Namespace}, pxc)
	if err != nil {
		if err = client.IgnoreNotFound(err); err != nil {
			return err
		}
	}
	if pxc.Spec.Pause != database.Spec.Paused {
		// During the restoration of PXC clusters
		// They need to be shutted down
		//
		// It's not a good idea to shutdown them from DatabaseCluster object perspective
		// hence we have this piece of the migration of spec.pause field
		// from PerconaXtraDBCluster object to a DatabaseCluster object.

		restores := &everestv1alpha1.DatabaseClusterRestoreList{}

		listOps := &client.ListOptions{
			FieldSelector: fields.OneTermEqualSelector(dbClusterRestoreDBClusterNameField, database.Name),
			Namespace:     database.Namespace,
		}
		err = r.List(ctx, restores, listOps)
		if err != nil {
			return err
		}
		for _, restore := range restores.Items {
			if !restore.IsComplete(database.Spec.Engine.Type) {
				database.Spec.Paused = pxc.Spec.Pause
				break
			}
		}
	}

	pxc.Name = database.Name
	pxc.Namespace = database.Namespace
	pxc.Annotations = database.Annotations

	if len(database.Finalizers) != 0 {
		pxc.Finalizers = database.Finalizers
		database.Finalizers = []string{}
	}

	if database.Spec.Proxy.Type == "" {
		database.Spec.Proxy.Type = everestv1alpha1.ProxyTypeHAProxy
	}
	if database.Spec.Engine.Config == "" {
		if database.Spec.Engine.Resources.Memory.Cmp(memorySmallSize) == 0 {
			database.Spec.Engine.Config = pxcConfigSizeSmall
		}
		if database.Spec.Engine.Resources.Memory.Cmp(memoryMediumSize) == 0 {
			database.Spec.Engine.Config = pxcConfigSizeMedium
		}
		if database.Spec.Engine.Resources.Memory.Cmp(memoryLargeSize) == 0 {
			database.Spec.Engine.Config = pxcConfigSizeLarge
		}
	}
	if err := r.Update(ctx, database); err != nil {
		return err
	}

	engine := &everestv1alpha1.DatabaseEngine{}
	err = r.Get(ctx, types.NamespacedName{Name: pxcDeploymentName, Namespace: database.Namespace}, engine)
	if err != nil {
		return errors.Join(err, fmt.Errorf("failed to get database engine %s", pxcDeploymentName))
	}

	if err := controllerutil.SetControllerReference(database, pxc, r.Client.Scheme()); err != nil {
		return err
	}
	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, pxc, func() error {
		if !controllerutil.ContainsFinalizer(pxc, finalizerDeletePXCPodsInOrder) {
			controllerutil.AddFinalizer(pxc, finalizerDeletePXCPodsInOrder)
		}
		if !controllerutil.ContainsFinalizer(pxc, finalizerDeletePXCPVC) {
			controllerutil.AddFinalizer(pxc, finalizerDeletePXCPVC)
		}
		if !controllerutil.ContainsFinalizer(pxc, finalizerDeletePXCSSL) {
			controllerutil.AddFinalizer(pxc, finalizerDeletePXCSSL)
		}
		pxc.TypeMeta = metav1.TypeMeta{
			APIVersion: version.ToAPIVersion(pxcAPIGroup),
			Kind:       PerconaXtraDBClusterKind,
		}

		crVersion := version.ToCRVersion()
		if pxc.Spec.CRVersion != "" {
			crVersion = pxc.Spec.CRVersion
		}
		clusterType, err := r.getClusterType(ctx)
		if err != nil {
			return err
		}

		pxc.Spec = *r.defaultPXCSpec()
		if clusterType == ClusterTypeEKS {
			affinity := &pxcv1.PodAffinity{
				TopologyKey: pointer.ToString("kubernetes.io/hostname"),
			}
			pxc.Spec.PXC.PodSpec.Affinity = affinity
			pxc.Spec.HAProxy.PodSpec.Affinity = affinity
			pxc.Spec.ProxySQL.Affinity = affinity
		}

		dbTemplateKind, hasTemplateKind := database.ObjectMeta.Annotations[dbTemplateKindAnnotationKey]
		dbTemplateName, hasTemplateName := database.ObjectMeta.Annotations[dbTemplateNameAnnotationKey]
		if hasTemplateKind && !hasTemplateName {
			return fmt.Errorf("missing %s annotation", dbTemplateNameAnnotationKey)
		}
		if !hasTemplateKind && hasTemplateName {
			return fmt.Errorf("missing %s annotation", dbTemplateKindAnnotationKey)
		}
		if hasTemplateKind && hasTemplateName {
			err := r.applyTemplate(ctx, pxc, dbTemplateKind, types.NamespacedName{
				Namespace: req.NamespacedName.Namespace,
				Name:      dbTemplateName,
			})
			if err != nil {
				return err
			}
		}

		pxc.Spec.CRVersion = crVersion
		pxc.Spec.AllowUnsafeConfig = database.Spec.AllowUnsafeConfiguration
		pxc.Spec.Pause = database.Spec.Paused
		pxc.Spec.SecretsName = database.Spec.Engine.UserSecretsName

		if database.Spec.Engine.Config != "" {
			pxc.Spec.PXC.PodSpec.Configuration = database.Spec.Engine.Config
		}

		pxc.Spec.PXC.PodSpec.Size = database.Spec.Engine.Replicas

		if database.Spec.Engine.Version == "" {
			database.Spec.Engine.Version = engine.BestEngineVersion()
		}
		pxcEngineVersion, ok := engine.Status.AvailableVersions.Engine[database.Spec.Engine.Version]
		if !ok {
			return fmt.Errorf("engine version %s not available", database.Spec.Engine.Version)
		}

		pxc.Spec.PXC.PodSpec.Image = pxcEngineVersion.ImagePath

		pxc.Spec.PXC.PodSpec.VolumeSpec = &pxcv1.VolumeSpec{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimSpec{
				StorageClassName: database.Spec.Engine.Storage.Class,
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: database.Spec.Engine.Storage.Size,
					},
				},
			},
		}

		if !database.Spec.Engine.Resources.CPU.IsZero() {
			pxc.Spec.PXC.PodSpec.Resources.Limits[corev1.ResourceCPU] = database.Spec.Engine.Resources.CPU
		}
		if !database.Spec.Engine.Resources.Memory.IsZero() {
			pxc.Spec.PXC.PodSpec.Resources.Limits[corev1.ResourceMemory] = database.Spec.Engine.Resources.Memory
		}

		// Set timeouts and resources based on the database size.
		if database.Spec.Engine.Resources.Memory.Cmp(memorySmallSize) == 0 {
			pxc.Spec.PXC.PodSpec.LivenessProbes.TimeoutSeconds = 450
			pxc.Spec.PXC.PodSpec.ReadinessProbes.TimeoutSeconds = 450
			pxc.Spec.PXC.PodSpec.Resources = pxcResourceRequirementsSmall
		}
		if database.Spec.Engine.Resources.Memory.Cmp(memoryMediumSize) == 0 {
			pxc.Spec.PXC.PodSpec.LivenessProbes.TimeoutSeconds = 451
			pxc.Spec.PXC.PodSpec.ReadinessProbes.TimeoutSeconds = 451
			pxc.Spec.PXC.PodSpec.Resources = pxcResourceRequirementsMedium
		}
		if database.Spec.Engine.Resources.Memory.Cmp(memoryLargeSize) == 0 {
			pxc.Spec.PXC.PodSpec.LivenessProbes.TimeoutSeconds = 600
			pxc.Spec.PXC.PodSpec.ReadinessProbes.TimeoutSeconds = 600
			pxc.Spec.PXC.PodSpec.Resources = pxcResourceRequirementsLarge
		}

		switch database.Spec.Proxy.Type {
		case everestv1alpha1.ProxyTypeHAProxy:
			pxc.Spec.ProxySQL.Enabled = false
			pxc.Spec.HAProxy, err = r.genPXCHAProxySpec(ctx, database, engine, clusterType)
			if err != nil {
				return err
			}
		case everestv1alpha1.ProxyTypeProxySQL:
			pxc.Spec.HAProxy.PodSpec.Enabled = false
			pxc.Spec.ProxySQL, err = r.genPXCProxySQLSpec(database, engine)
			if err != nil {
				return err
			}
		default:
			return fmt.Errorf("invalid proxy type %s", database.Spec.Proxy.Type)
		}

		monitoring := &everestv1alpha1.MonitoringConfig{}
		if database.Spec.Monitoring != nil && database.Spec.Monitoring.MonitoringConfigName != "" {
			err := r.Get(ctx, types.NamespacedName{
				Namespace: r.monitoringNamespace,
				Name:      database.Spec.Monitoring.MonitoringConfigName,
			}, monitoring)
			if err != nil {
				return err
			}
			if !monitoring.IsNamespaceAllowed(database.Namespace) {
				return fmt.Errorf("%s namespace is not allowed to use for %s monitoring config", database.Namespace, monitoring.Name)
			}
		}

		if monitoring.Spec.Type == everestv1alpha1.PMMMonitoringType { //nolint:nestif
			image := defaultPMMClientImage
			if monitoring.Spec.PMM.Image != "" {
				image = monitoring.Spec.PMM.Image
			}

			//nolint:godox
			// TODO (K8SPXC-1367): Set PMM container LivenessProbes timeouts once possible.

			pxc.Spec.PMM.Enabled = true
			pmmURL, err := url.Parse(monitoring.Spec.PMM.URL)
			if err != nil {
				return errors.Join(err, errors.New("invalid monitoring URL"))
			}
			pxc.Spec.PMM.ServerHost = pmmURL.Hostname()
			pxc.Spec.PMM.Image = image
			pxc.Spec.PMM.Resources = database.Spec.Monitoring.Resources
			// Set resources based on cluster size.
			if database.Spec.Engine.Resources.Memory.Cmp(memorySmallSize) == 0 {
				pxc.Spec.PMM.Resources = pmmResourceRequirementsSmall
			}
			if database.Spec.Engine.Resources.Memory.Cmp(memoryMediumSize) == 0 {
				pxc.Spec.PMM.Resources = pmmResourceRequirementsMedium
			}
			if database.Spec.Engine.Resources.Memory.Cmp(memoryLargeSize) == 0 {
				pxc.Spec.PMM.Resources = pmmResourceRequirementsLarge
			}

			apiKey, err := r.getSecretFromMonitoringConfig(ctx, monitoring)
			if err != nil {
				return err
			}

			err = r.updateSecretData(ctx, database, pxc.Spec.SecretsName, map[string][]byte{
				"pmmserverkey": []byte(apiKey),
			})
			// If the secret does not exist, we need to wait for the PXC
			// operator to create it. If the secret already exists when the
			// cluster is initialized the PXC operator doesn't generate the
			// missing fields.
			if err != nil && !k8serrors.IsNotFound(err) {
				return err
			}
		}

		pxc.Spec.Backup, err = r.genPXCBackupSpec(ctx, database, engine)
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}

	if database.Spec.DataSource != nil && database.Status.Status == everestv1alpha1.AppStateReady {
		err = r.reconcileDBRestoreFromDataSource(ctx, database)
		if err != nil {
			return err
		}
	}

	database.Status.Status = everestv1alpha1.AppState(pxc.Status.Status)
	// If a restore is running for this database, set the database status to
	// restoring
	restoreList := &everestv1alpha1.DatabaseClusterRestoreList{}
	listOps := &client.ListOptions{
		FieldSelector: fields.OneTermEqualSelector(dbClusterRestoreDBClusterNameField, database.Name),
		Namespace:     database.Namespace,
	}
	err = r.List(ctx, restoreList, listOps)
	if err != nil {
		return errors.Join(err, errors.New("failed to list DatabaseClusterRestore objects"))
	}
	for _, restore := range restoreList.Items {
		if !restore.IsComplete(database.Spec.Engine.Type) {
			database.Status.Status = everestv1alpha1.AppStateRestoring
			break
		}
	}

	database.Status.Hostname = pxc.Status.Host
	database.Status.Ready = pxc.Status.Ready
	database.Status.Size = pxc.Status.Size
	database.Status.Message = strings.Join(pxc.Status.Messages, ";")
	database.Status.Port = 3306
	return r.Status().Update(ctx, database)
}

// createPGBackrestSecret creates or updates the PG Backrest secret.
func (r *DatabaseClusterReconciler) createPGBackrestSecret(
	ctx context.Context,
	database *everestv1alpha1.DatabaseCluster,
	pgbackrestKey string,
	pgbackrestConf []byte,
	pgBackrestSecretName string,
	additionalSecrets map[string][]byte,
) (*corev1.Secret, error) {
	pgBackrestSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pgBackrestSecretName,
			Namespace: database.Namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			pgbackrestKey: pgbackrestConf,
		},
	}

	for k, v := range additionalSecrets {
		pgBackrestSecret.Data[k] = v
	}

	err := controllerutil.SetControllerReference(database, pgBackrestSecret, r.Scheme)
	if err != nil {
		return nil, err
	}
	err = r.createOrUpdate(ctx, pgBackrestSecret, false)
	if err != nil {
		return nil, err
	}

	return pgBackrestSecret, nil
}

// getBackupStorageIndexInPGBackrestRepo returns the index of the backup storage in the pgbackrest repo list.
func getBackupStorageIndexInPGBackrestRepo(
	backupStorage *everestv1alpha1.BackupStorage,
	repos []crunchyv1beta1.PGBackRestRepo,
) int {
	for idx, repo := range repos {
		if repo.S3 != nil &&
			repo.S3.Bucket == backupStorage.Spec.Bucket &&
			repo.S3.Region == backupStorage.Spec.Region &&
			repo.S3.Endpoint == backupStorage.Spec.EndpointURL {
			return idx
		}

		if repo.Azure != nil && repo.Azure.Container == backupStorage.Spec.Bucket {
			return idx
		}
	}

	return -1
}

// genPGBackrestRepo generates a PGBackrest repo for a given backup storage.
func genPGBackrestRepo(
	repoName string,
	backupStorage everestv1alpha1.BackupStorageSpec,
	backupSchedule *string,
) (crunchyv1beta1.PGBackRestRepo, error) {
	pgRepo := crunchyv1beta1.PGBackRestRepo{
		Name: repoName,
	}

	if backupSchedule != nil {
		pgRepo.BackupSchedules = &crunchyv1beta1.PGBackRestBackupSchedules{
			Full: backupSchedule,
		}
	}

	switch backupStorage.Type {
	case everestv1alpha1.BackupStorageTypeS3:
		pgRepo.S3 = &crunchyv1beta1.RepoS3{
			Bucket:   backupStorage.Bucket,
			Region:   backupStorage.Region,
			Endpoint: backupStorage.EndpointURL,
		}
	case everestv1alpha1.BackupStorageTypeAzure:
		pgRepo.Azure = &crunchyv1beta1.RepoAzure{
			Container: backupStorage.Bucket,
		}
	default:
		return crunchyv1beta1.PGBackRestRepo{}, fmt.Errorf("unsupported backup storage type %s", backupStorage.Type)
	}

	return pgRepo, nil
}

// Adds the backup storage credentials to the PGBackrest secret data.
func addBackupStorageCredentialsToPGBackrestSecretIni(
	storageType everestv1alpha1.BackupStorageType,
	cfg *ini.File,
	repoName string,
	secret *corev1.Secret,
) error {
	switch storageType {
	case everestv1alpha1.BackupStorageTypeS3:
		_, err := cfg.Section("global").NewKey(
			repoName+"-s3-key",
			string(secret.Data["AWS_ACCESS_KEY_ID"]),
		)
		if err != nil {
			return err
		}

		_, err = cfg.Section("global").NewKey(
			repoName+"-s3-key-secret",
			string(secret.Data["AWS_SECRET_ACCESS_KEY"]),
		)
		if err != nil {
			return err
		}
	case everestv1alpha1.BackupStorageTypeAzure:
		_, err := cfg.Section("global").NewKey(
			repoName+"-azure-account",
			string(secret.Data["AZURE_STORAGE_ACCOUNT_NAME"]),
		)
		if err != nil {
			return err
		}

		_, err = cfg.Section("global").NewKey(
			repoName+"-azure-key",
			string(secret.Data["AZURE_STORAGE_ACCOUNT_KEY"]),
		)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("backup storage type %s not supported", storageType)
	}

	return nil
}

func backupStorageNameFromRepo(backupStorages map[string]everestv1alpha1.BackupStorageSpec, repo crunchyv1beta1.PGBackRestRepo) string {
	for name, spec := range backupStorages {
		if repo.S3 != nil &&
			repo.S3.Bucket == spec.Bucket &&
			repo.S3.Region == spec.Region &&
			repo.S3.Endpoint == spec.EndpointURL {
			return name
		}

		if repo.Azure != nil && repo.Azure.Container == spec.Bucket {
			return name
		}
	}

	return ""
}

func removeString(s []string, i int) []string {
	return append(s[:i], s[i+1:]...)
}

func removeRepoName(s []string, n string) []string {
	for i, name := range s {
		if name == n {
			return removeString(s, i)
		}
	}
	return s
}

func backupStorageTypeFromBackrestRepo(repo crunchyv1beta1.PGBackRestRepo) (everestv1alpha1.BackupStorageType, error) {
	if repo.S3 != nil {
		return everestv1alpha1.BackupStorageTypeS3, nil
	}

	if repo.Azure != nil {
		return everestv1alpha1.BackupStorageTypeAzure, nil
	}

	return "", errors.New("could not determine backup storage type from repo")
}

//nolint:gocognit,gocyclo,cyclop,maintidx
func reconcilePGBackRestRepos(
	oldRepos []crunchyv1beta1.PGBackRestRepo,
	backupSchedules []everestv1alpha1.BackupSchedule,
	backupRequests []everestv1alpha1.DatabaseClusterBackup,
	backupStorages map[string]everestv1alpha1.BackupStorageSpec,
	backupStoragesSecrets map[string]*corev1.Secret,
	engineStorage everestv1alpha1.Storage,
	db *everestv1alpha1.DatabaseCluster,
) (
	[]crunchyv1beta1.PGBackRestRepo,
	map[string]string,
	[]byte,
	error,
) {
	backupStoragesInRepos := map[string]struct{}{}
	pgBackRestGlobal := map[string]string{}
	pgBackRestSecretIni, err := ini.Load([]byte{})
	if err != nil {
		return []crunchyv1beta1.PGBackRestRepo{}, map[string]string{}, []byte{}, errors.Join(err, errors.New("failed to initialize PGBackrest secret data"))
	}

	availableRepoNames := []string{
		// repo1 is reserved for the PVC-based repo, see below for details on
		// how it is handled
		"repo2",
		"repo3",
		"repo4",
	}

	reposReconciled := list.New()
	reposToBeReconciled := list.New()
	for _, repo := range oldRepos {
		reposToBeReconciled.PushBack(repo)
	}
	backupSchedulesReconciled := list.New()
	backupSchedulesToBeReconciled := list.New()
	for _, backupSchedule := range backupSchedules {
		if !backupSchedule.Enabled {
			continue
		}
		backupSchedulesToBeReconciled.PushBack(backupSchedule)
	}

	// Move repos with set schedules which are already correct to the
	// reconciled list
	var ebNext *list.Element
	var erNext *list.Element
	for eb := backupSchedulesToBeReconciled.Front(); eb != nil; eb = ebNext {
		// Save the next element because we might remove the current one
		ebNext = eb.Next()

		backupSchedule, ok := eb.Value.(everestv1alpha1.BackupSchedule)
		if !ok {
			return []crunchyv1beta1.PGBackRestRepo{}, map[string]string{}, []byte{}, fmt.Errorf("failed to cast backup schedule %v", eb.Value)
		}
		for er := reposToBeReconciled.Front(); er != nil; er = erNext {
			// Save the next element because we might remove the current one
			erNext = er.Next()

			repo, ok := er.Value.(crunchyv1beta1.PGBackRestRepo)
			if !ok {
				return []crunchyv1beta1.PGBackRestRepo{}, map[string]string{}, []byte{}, fmt.Errorf("failed to cast repo %v", er.Value)
			}
			repoBackupStorageName := backupStorageNameFromRepo(backupStorages, repo)
			if backupSchedule.BackupStorageName != repoBackupStorageName ||
				repo.BackupSchedules == nil ||
				repo.BackupSchedules.Full == nil ||
				backupSchedule.Schedule != *repo.BackupSchedules.Full {
				continue
			}

			reposReconciled.PushBack(repo)
			reposToBeReconciled.Remove(er)
			backupSchedulesReconciled.PushBack(backupSchedule)
			backupSchedulesToBeReconciled.Remove(eb)
			availableRepoNames = removeRepoName(availableRepoNames, repo.Name)

			// Keep track of backup storages which are already in use by a repo
			backupStoragesInRepos[backupSchedule.BackupStorageName] = struct{}{}

			pgBackRestGlobal[fmt.Sprintf(pgBackRestPathTmpl, repo.Name)] = "/" + backupStoragePrefix(db)
			pgBackRestGlobal[fmt.Sprintf(pgBackRestRetentionTmpl, repo.Name)] = strconv.Itoa(getPGRetentionCopies(backupSchedule.RetentionCopies))
			sType, err := backupStorageTypeFromBackrestRepo(repo)
			if err != nil {
				return []crunchyv1beta1.PGBackRestRepo{},
					map[string]string{},
					[]byte{},
					err
			}

			if verify := pointer.Get(backupStorages[repo.Name].VerifyTLS); !verify {
				// See: https://pgbackrest.org/configuration.html#section-repository/option-repo-storage-verify-tls
				pgBackRestGlobal[fmt.Sprintf(pgBackRestStorageVerifyTmpl, repo.Name)] = "n"
			}

			err = addBackupStorageCredentialsToPGBackrestSecretIni(
				sType, pgBackRestSecretIni, repo.Name, backupStoragesSecrets[repoBackupStorageName])
			if err != nil {
				return []crunchyv1beta1.PGBackRestRepo{},
					map[string]string{},
					[]byte{},
					errors.Join(err, errors.New("failed to add backup storage credentials to PGBackrest secret data"))
			}

			break
		}
	}

	// Update the schedules of the repos which already exist but have the wrong
	// schedule and move them to the reconciled list
	for er := reposToBeReconciled.Front(); er != nil; er = erNext {
		// Save the next element because we might remove the current one
		erNext = er.Next()

		repo, ok := er.Value.(crunchyv1beta1.PGBackRestRepo)
		if !ok {
			return []crunchyv1beta1.PGBackRestRepo{}, map[string]string{}, []byte{}, fmt.Errorf("failed to cast repo %v", er.Value)
		}
		repoBackupStorageName := backupStorageNameFromRepo(backupStorages, repo)
		for eb := backupSchedulesToBeReconciled.Front(); eb != nil; eb = ebNext {
			// Save the next element because we might remove the current one
			ebNext = eb.Next()

			backupSchedule, ok := eb.Value.(everestv1alpha1.BackupSchedule)
			if !ok {
				return []crunchyv1beta1.PGBackRestRepo{}, map[string]string{}, []byte{}, fmt.Errorf("failed to cast backup schedule %v", eb.Value)
			}
			if backupSchedule.BackupStorageName == repoBackupStorageName {
				repo.BackupSchedules = &crunchyv1beta1.PGBackRestBackupSchedules{
					Full: &backupSchedule.Schedule,
				}

				reposReconciled.PushBack(repo)
				reposToBeReconciled.Remove(er)
				backupSchedulesReconciled.PushBack(backupSchedule)
				backupSchedulesToBeReconciled.Remove(eb)
				availableRepoNames = removeRepoName(availableRepoNames, repo.Name)

				// Keep track of backup storages which are already in use by a repo
				backupStoragesInRepos[backupSchedule.BackupStorageName] = struct{}{}

				pgBackRestGlobal[fmt.Sprintf(pgBackRestPathTmpl, repo.Name)] = "/" + backupStoragePrefix(db)
				pgBackRestGlobal[fmt.Sprintf(pgBackRestRetentionTmpl, repo.Name)] = strconv.Itoa(getPGRetentionCopies(backupSchedule.RetentionCopies))

				if verify := pointer.Get(backupStorages[repo.Name].VerifyTLS); !verify {
					// See: https://pgbackrest.org/configuration.html#section-repository/option-repo-storage-verify-tls
					pgBackRestGlobal[fmt.Sprintf(pgBackRestStorageVerifyTmpl, repo.Name)] = "n"
				}

				sType, err := backupStorageTypeFromBackrestRepo(repo)
				if err != nil {
					return []crunchyv1beta1.PGBackRestRepo{},
						map[string]string{},
						[]byte{},
						err
				}

				err = addBackupStorageCredentialsToPGBackrestSecretIni(sType, pgBackRestSecretIni, repo.Name, backupStoragesSecrets[repoBackupStorageName])
				if err != nil {
					return []crunchyv1beta1.PGBackRestRepo{},
						map[string]string{},
						[]byte{},
						errors.Join(err, errors.New("failed to add backup storage credentials to PGBackrest secret data"))
				}

				break
			}
		}
	}

	// Add new backup schedules
	for eb := backupSchedulesToBeReconciled.Front(); eb != nil; eb = eb.Next() {
		backupSchedule, ok := eb.Value.(everestv1alpha1.BackupSchedule)
		if !ok {
			return []crunchyv1beta1.PGBackRestRepo{}, map[string]string{}, []byte{}, fmt.Errorf("failed to cast backup schedule %v", eb.Value)
		}
		backupStorage, ok := backupStorages[backupSchedule.BackupStorageName]
		if !ok {
			return []crunchyv1beta1.PGBackRestRepo{}, map[string]string{}, []byte{}, fmt.Errorf("unknown backup storage %s", backupSchedule.BackupStorageName)
		}

		if len(availableRepoNames) == 0 {
			return []crunchyv1beta1.PGBackRestRepo{}, map[string]string{}, []byte{}, errors.New("exceeded max number of repos")
		}

		repoName := availableRepoNames[0]
		availableRepoNames = removeRepoName(availableRepoNames, repoName)
		repo, err := genPGBackrestRepo(repoName, backupStorage, &backupSchedule.Schedule)
		if err != nil {
			return []crunchyv1beta1.PGBackRestRepo{}, map[string]string{}, []byte{}, err
		}
		reposReconciled.PushBack(repo)

		// Keep track of backup storages which are already in use by a repo
		backupStoragesInRepos[backupSchedule.BackupStorageName] = struct{}{}

		if verify := pointer.Get(backupStorage.VerifyTLS); !verify {
			// See: https://pgbackrest.org/configuration.html#section-repository/option-repo-storage-verify-tls
			pgBackRestGlobal[fmt.Sprintf(pgBackRestStorageVerifyTmpl, repo.Name)] = "n"
		}
		pgBackRestGlobal[fmt.Sprintf(pgBackRestPathTmpl, repo.Name)] = "/" + backupStoragePrefix(db)
		pgBackRestGlobal[fmt.Sprintf(pgBackRestRetentionTmpl, repo.Name)] = strconv.Itoa(getPGRetentionCopies(backupSchedule.RetentionCopies))
		sType, err := backupStorageTypeFromBackrestRepo(repo)
		if err != nil {
			return []crunchyv1beta1.PGBackRestRepo{},
				map[string]string{},
				[]byte{},
				err
		}

		err = addBackupStorageCredentialsToPGBackrestSecretIni(sType, pgBackRestSecretIni, repo.Name, backupStoragesSecrets[backupSchedule.BackupStorageName])
		if err != nil {
			return []crunchyv1beta1.PGBackRestRepo{},
				map[string]string{},
				[]byte{},
				errors.Join(err, errors.New("failed to add backup storage credentials to PGBackrest secret data"))
		}
	}

	// Add on-demand backups whose backup storage doesn't have a schedule
	// defined
	// XXX some of these backups might be fake, see the XXX comment in the
	// reconcilePGBackupsSpec function for more context. Be very careful if you
	// decide to use fields other that the Spec.BackupStorageName.
	for _, backupRequest := range backupRequests {
		// Backup storage already in repos, skip
		if _, ok := backupStoragesInRepos[backupRequest.Spec.BackupStorageName]; ok {
			continue
		}

		backupStorage, ok := backupStorages[backupRequest.Spec.BackupStorageName]
		if !ok {
			return []crunchyv1beta1.PGBackRestRepo{}, map[string]string{}, []byte{}, fmt.Errorf("unknown backup storage %s", backupRequest.Spec.BackupStorageName)
		}

		if len(availableRepoNames) == 0 {
			return []crunchyv1beta1.PGBackRestRepo{}, map[string]string{}, []byte{}, errors.New("exceeded max number of repos")
		}

		repoName := availableRepoNames[0]
		availableRepoNames = removeRepoName(availableRepoNames, repoName)
		repo, err := genPGBackrestRepo(repoName, backupStorage, nil)
		if err != nil {
			return []crunchyv1beta1.PGBackRestRepo{}, map[string]string{}, []byte{}, err
		}
		reposReconciled.PushBack(repo)

		// Keep track of backup storages which are already in use by a repo
		backupStoragesInRepos[backupRequest.Spec.BackupStorageName] = struct{}{}

		if verify := pointer.Get(backupStorage.VerifyTLS); !verify {
			// See: https://pgbackrest.org/configuration.html#section-repository/option-repo-storage-verify-tls
			pgBackRestGlobal[fmt.Sprintf(pgBackRestStorageVerifyTmpl, repo.Name)] = "n"
		}
		pgBackRestGlobal[fmt.Sprintf(pgBackRestPathTmpl, repo.Name)] = "/" + backupStoragePrefix(db)
		sType, err := backupStorageTypeFromBackrestRepo(repo)
		if err != nil {
			return []crunchyv1beta1.PGBackRestRepo{},
				map[string]string{},
				[]byte{},
				err
		}

		err = addBackupStorageCredentialsToPGBackrestSecretIni(sType, pgBackRestSecretIni, repo.Name, backupStoragesSecrets[backupRequest.Spec.BackupStorageName])
		if err != nil {
			return []crunchyv1beta1.PGBackRestRepo{},
				map[string]string{},
				[]byte{},
				errors.Join(err, errors.New("failed to add backup storage credentials to PGBackrest secret data"))
		}
	}

	// The PG operator requires a repo to be set up in order to create
	// replicas. Without any credentials we can't set a cloud-based repo so we
	// define a PVC-backed repo in case the user doesn't define any cloud-based
	// repos. Moreover, we need to keep this repo in the list even if the user
	// defines a cloud-based repo because the PG operator will restart the
	// cluster if the only repo in the list is changed.
	newRepos := []crunchyv1beta1.PGBackRestRepo{
		{
			Name: "repo1",
			Volume: &crunchyv1beta1.RepoPVC{
				VolumeClaimSpec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteOnce,
					},
					StorageClassName: engineStorage.Class,
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: engineStorage.Size,
						},
					},
				},
			},
		},
	}
	pgBackRestGlobal["repo1-retention-full"] = "1"

	// Add the reconciled repos to the list of repos
	for e := reposReconciled.Front(); e != nil; e = e.Next() {
		repo, ok := e.Value.(crunchyv1beta1.PGBackRestRepo)
		if !ok {
			return []crunchyv1beta1.PGBackRestRepo{}, map[string]string{}, []byte{}, fmt.Errorf("failed to cast repo %v", e.Value)
		}
		newRepos = append(newRepos, repo)
	}

	pgBackrestSecretBuf := new(bytes.Buffer)
	if _, err = pgBackRestSecretIni.WriteTo(pgBackrestSecretBuf); err != nil {
		return []crunchyv1beta1.PGBackRestRepo{}, map[string]string{}, []byte{}, errors.Join(err, errors.New("failed to write PGBackrest secret data"))
	}

	return newRepos, pgBackRestGlobal, pgBackrestSecretBuf.Bytes(), nil
}

//nolint:gocognit
func (r *DatabaseClusterReconciler) reconcilePGBackupsSpec( //nolint:maintidx
	ctx context.Context,
	oldBackups crunchyv1beta1.Backups,
	database *everestv1alpha1.DatabaseCluster,
	engine *everestv1alpha1.DatabaseEngine,
) (crunchyv1beta1.Backups, error) {
	pgbackrestVersion, ok := engine.Status.AvailableVersions.Backup[database.Spec.Engine.Version]
	if !ok {
		return crunchyv1beta1.Backups{}, fmt.Errorf("pgbackrest version %s not available", database.Spec.Engine.Version)
	}

	newBackups := crunchyv1beta1.Backups{
		PGBackRest: crunchyv1beta1.PGBackRestArchive{
			Image:   pgbackrestVersion.ImagePath,
			Manual:  oldBackups.PGBackRest.Manual,
			Restore: oldBackups.PGBackRest.Restore,
		},
	}

	if newBackups.PGBackRest.Manual == nil {
		// This field is required by the operator, but it doesn't impact
		// our manual backup operation because we use the PerconaPGBackup
		// CR to request on-demand backups which then changes the Manual field
		// internally and sets the
		// postgres-operator.crunchydata.com/pgbackrest-backup annotation.
		newBackups.PGBackRest.Manual = &crunchyv1beta1.PGBackRestManualBackup{
			RepoName: "repo1",
		}
	}

	// List DatabaseClusterBackup objects for this database
	backupList := &everestv1alpha1.DatabaseClusterBackupList{}
	listOps := &client.ListOptions{
		FieldSelector: fields.OneTermEqualSelector(dbClusterBackupDBClusterNameField, database.Name),
		Namespace:     database.Namespace,
	}
	err := r.List(ctx, backupList, listOps)
	if err != nil {
		return crunchyv1beta1.Backups{}, errors.Join(err, errors.New("failed to list DatabaseClusterBackup objects"))
	}

	backupStorages := map[string]everestv1alpha1.BackupStorageSpec{}
	backupStoragesSecrets := map[string]*corev1.Secret{}
	// Add backup storages used by on-demand backups to the list
	for _, backup := range backupList.Items {
		// Check if we already fetched that backup storage
		if _, ok := backupStorages[backup.Spec.BackupStorageName]; ok {
			continue
		}

		backupStorage := &everestv1alpha1.BackupStorage{}
		err := r.Get(ctx, types.NamespacedName{Name: backup.Spec.BackupStorageName, Namespace: r.systemNamespace}, backupStorage)
		if err != nil {
			return crunchyv1beta1.Backups{}, errors.Join(err, fmt.Errorf("failed to get backup storage %s", backup.Spec.BackupStorageName))
		}
		if database.Namespace != r.systemNamespace {
			if err := r.reconcileBackupStorageSecret(ctx, backupStorage, database); err != nil {
				return crunchyv1beta1.Backups{}, err
			}
		}

		backupStorageSecret := &corev1.Secret{}
		err = r.Get(ctx, types.NamespacedName{Name: backupStorage.Spec.CredentialsSecretName, Namespace: backupStorage.Namespace}, backupStorageSecret)
		if err != nil {
			return crunchyv1beta1.Backups{}, errors.Join(err, fmt.Errorf("failed to get backup storage secret %s", backupStorage.Spec.CredentialsSecretName))
		}

		backupStorages[backupStorage.Name] = backupStorage.Spec
		backupStoragesSecrets[backupStorage.Name] = backupStorageSecret
	}

	// List DatabaseClusterRestore objects for this database
	restoreList := &everestv1alpha1.DatabaseClusterRestoreList{}
	listOps = &client.ListOptions{
		FieldSelector: fields.OneTermEqualSelector(dbClusterRestoreDBClusterNameField, database.Name),
		Namespace:     database.Namespace,
	}
	err = r.List(ctx, restoreList, listOps)
	if err != nil {
		return crunchyv1beta1.Backups{}, errors.Join(err, errors.New("failed to list DatabaseClusterRestore objects"))
	}

	// Add backup storages used by restores to the list
	for _, restore := range restoreList.Items {
		// If the restore has already completed, skip it.
		if restore.IsComplete(database.Spec.Engine.Type) {
			continue
		}

		var backupStorageName string
		if restore.Spec.DataSource.DBClusterBackupName != "" {
			backup := &everestv1alpha1.DatabaseClusterBackup{}
			err := r.Get(ctx, types.NamespacedName{Name: restore.Spec.DataSource.DBClusterBackupName, Namespace: restore.Namespace}, backup)
			if err != nil {
				return crunchyv1beta1.Backups{}, errors.Join(err, errors.New("failed to get DatabaseClusterBackup"))
			}

			backupStorageName = backup.Spec.BackupStorageName
		}
		if restore.Spec.DataSource.BackupSource != nil {
			backupStorageName = restore.Spec.DataSource.BackupSource.BackupStorageName
		}

		// Check if we already fetched that backup storage
		if _, ok := backupStorages[backupStorageName]; ok {
			continue
		}

		backupStorage := &everestv1alpha1.BackupStorage{}
		err := r.Get(ctx, types.NamespacedName{Name: backupStorageName, Namespace: r.systemNamespace}, backupStorage)
		if err != nil {
			return crunchyv1beta1.Backups{}, errors.Join(err, fmt.Errorf("failed to get backup storage %s", backupStorageName))
		}
		if database.Namespace != r.systemNamespace {
			if err := r.reconcileBackupStorageSecret(ctx, backupStorage, database); err != nil {
				return crunchyv1beta1.Backups{}, err
			}
		}

		backupStorageSecret := &corev1.Secret{}
		err = r.Get(ctx, types.NamespacedName{Name: backupStorage.Spec.CredentialsSecretName, Namespace: backupStorage.Namespace}, backupStorageSecret)
		if err != nil {
			return crunchyv1beta1.Backups{}, errors.Join(err, fmt.Errorf("failed to get backup storage secret %s", backupStorage.Spec.CredentialsSecretName))
		}

		backupStorages[backupStorage.Name] = backupStorage.Spec
		backupStoragesSecrets[backupStorage.Name] = backupStorageSecret

		// XXX We need to add this restore's backup storage to the list of
		// repos.
		// Passing the restoreList to the reconcilePGBackRestRepos function is
		// not a good option because in that case the
		// reconcilePGBackRestRepos function would no longer be purely
		// functional, as it would then need to fetch the DatabaseClusterBackup
		// object referenced by the restore from the API server.
		// We could instead create a new restore type that would also include
		// the backup storage and fetch it here, but that seems a bit overkill.
		// We already pass on the backupList to the reconcilePGBackRestRepos
		// function that only uses the DatabaseClusterBackup reference to the
		// backup storage name which is exactly what we need here too. Let's
		// just add a fake backup to the backupList so that the restore's
		// backup storage is included in the repos.
		backupList.Items = append(backupList.Items, everestv1alpha1.DatabaseClusterBackup{
			Spec: everestv1alpha1.DatabaseClusterBackupSpec{
				BackupStorageName: backupStorageName,
			},
		})
	}

	// Only use the backup schedules if schedules are enabled in the DBC spec
	backupSchedules := []everestv1alpha1.BackupSchedule{}
	if database.Spec.Backup.Enabled {
		backupSchedules = database.Spec.Backup.Schedules
	}

	// Add backup storages used by backup schedules to the list
	for _, schedule := range backupSchedules {
		// Check if we already fetched that backup storage
		if _, ok := backupStorages[schedule.BackupStorageName]; ok {
			continue
		}

		backupStorage := &everestv1alpha1.BackupStorage{}
		err := r.Get(ctx, types.NamespacedName{Name: schedule.BackupStorageName, Namespace: r.systemNamespace}, backupStorage)
		if err != nil {
			return crunchyv1beta1.Backups{}, errors.Join(err, fmt.Errorf("failed to get backup storage %s", schedule.BackupStorageName))
		}
		if database.Namespace != r.systemNamespace {
			if err := r.reconcileBackupStorageSecret(ctx, backupStorage, database); err != nil {
				return crunchyv1beta1.Backups{}, err
			}
		}

		backupStorageSecret := &corev1.Secret{}
		err = r.Get(ctx, types.NamespacedName{Name: backupStorage.Spec.CredentialsSecretName, Namespace: backupStorage.Namespace}, backupStorageSecret)
		if err != nil {
			return crunchyv1beta1.Backups{}, errors.Join(err, fmt.Errorf("failed to get backup storage secret %s", backupStorage.Spec.CredentialsSecretName))
		}

		backupStorages[backupStorage.Name] = backupStorage.Spec
		backupStoragesSecrets[backupStorage.Name] = backupStorageSecret
	}

	pgBackrestRepos, pgBackrestGlobal, pgBackrestSecretData, err := reconcilePGBackRestRepos(
		oldBackups.PGBackRest.Repos,
		backupSchedules,
		backupList.Items,
		backupStorages,
		backupStoragesSecrets,
		database.Spec.Engine.Storage,
		database,
	)
	if err != nil {
		return crunchyv1beta1.Backups{}, err
	}

	pgBackrestSecret, err := r.createPGBackrestSecret(
		ctx,
		database,
		"s3.conf",
		pgBackrestSecretData,
		database.Name+"-pgbackrest-secrets",
		nil,
	)
	if err != nil {
		return crunchyv1beta1.Backups{}, err
	}

	newBackups.PGBackRest.Configuration = []corev1.VolumeProjection{
		{
			Secret: &corev1.SecretProjection{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: pgBackrestSecret.Name,
				},
			},
		},
	}

	newBackups.PGBackRest.Repos = pgBackrestRepos
	newBackups.PGBackRest.Global = pgBackrestGlobal
	return newBackups, nil
}

func (r *DatabaseClusterReconciler) genPGDataSourceSpec(ctx context.Context, database *everestv1alpha1.DatabaseCluster) (*crunchyv1beta1.DataSource, error) { //nolint:gocognit,lll
	if (database.Spec.DataSource.DBClusterBackupName == "" && database.Spec.DataSource.BackupSource == nil) ||
		(database.Spec.DataSource.DBClusterBackupName != "" && database.Spec.DataSource.BackupSource != nil) {
		return nil, errors.New("either DBClusterBackupName or BackupSource must be specified in the DataSource field")
	}

	var backupBaseName string
	var backupStorageName string
	var dest string
	if database.Spec.DataSource.DBClusterBackupName != "" {
		dbClusterBackup := &everestv1alpha1.DatabaseClusterBackup{}
		err := r.Get(ctx, types.NamespacedName{Name: database.Spec.DataSource.DBClusterBackupName, Namespace: database.Namespace}, dbClusterBackup)
		if err != nil {
			return nil, errors.Join(err, fmt.Errorf("failed to get DBClusterBackup %s", database.Spec.DataSource.DBClusterBackupName))
		}

		if dbClusterBackup.Status.Destination == nil {
			return nil, fmt.Errorf("DBClusterBackup %s has no destination", database.Spec.DataSource.DBClusterBackupName)
		}

		backupBaseName = filepath.Base(*dbClusterBackup.Status.Destination)
		backupStorageName = dbClusterBackup.Spec.BackupStorageName
		dest = *dbClusterBackup.Status.Destination
	}

	if database.Spec.DataSource.BackupSource != nil {
		backupBaseName = filepath.Base(database.Spec.DataSource.BackupSource.Path)
		backupStorageName = database.Spec.DataSource.BackupSource.BackupStorageName
	}

	backupStorage := &everestv1alpha1.BackupStorage{}
	err := r.Get(ctx, types.NamespacedName{Name: backupStorageName, Namespace: r.systemNamespace}, backupStorage)
	if err != nil {
		return nil, errors.Join(err, fmt.Errorf("failed to get backup storage %s", backupStorageName))
	}
	if database.Namespace != r.systemNamespace {
		if err := r.reconcileBackupStorageSecret(ctx, backupStorage, database); err != nil {
			return nil, err
		}
	}

	repoName := "repo1"
	options, err := getPGRestoreOptions(*database.Spec.DataSource, backupBaseName)
	if err != nil {
		return nil, err
	}

	pgDataSource := &crunchyv1beta1.DataSource{
		PGBackRest: &crunchyv1beta1.PGBackRestDataSource{
			Global: map[string]string{
				fmt.Sprintf(pgBackRestPathTmpl, repoName): globalDatasourceDestination(dest, database, backupStorage),
			},
			Stanza:  "db",
			Options: options,
			// XXX: Remove this once templates will be available
			Resources: corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("128Mi"),
					corev1.ResourceCPU:    resource.MustParse("200m"),
				},
			},
		},
	}

	switch backupStorage.Spec.Type {
	case everestv1alpha1.BackupStorageTypeS3:
		pgBackRestSecretIni, err := ini.Load([]byte{})
		if err != nil {
			return nil, errors.Join(err, errors.New("failed to initialize PGBackrest secret data"))
		}

		backupStorageSecret := &corev1.Secret{}
		err = r.Get(ctx, types.NamespacedName{Name: backupStorage.Spec.CredentialsSecretName, Namespace: backupStorage.Namespace}, backupStorageSecret)
		if err != nil {
			return nil, errors.Join(err, fmt.Errorf("failed to get backup storage secret %s", backupStorage.Spec.CredentialsSecretName))
		}

		err = addBackupStorageCredentialsToPGBackrestSecretIni(everestv1alpha1.BackupStorageTypeS3, pgBackRestSecretIni, repoName, backupStorageSecret)
		if err != nil {
			return nil, errors.Join(err, errors.New("failed to add data source storage credentials to PGBackrest secret data"))
		}
		pgBackrestSecretBuf := new(bytes.Buffer)
		if _, err = pgBackRestSecretIni.WriteTo(pgBackrestSecretBuf); err != nil {
			return nil, errors.Join(err, errors.New("failed to write PGBackrest secret data"))
		}

		secretData := pgBackrestSecretBuf.Bytes()

		pgBackrestSecret, err := r.createPGBackrestSecret(
			ctx,
			database,
			"s3.conf",
			secretData,
			database.Name+"-pgbackrest-datasource-secrets",
			nil,
		)
		if err != nil {
			return nil, errors.Join(err, errors.New("failed to create pgbackrest secret"))
		}

		pgDataSource.PGBackRest.Configuration = []corev1.VolumeProjection{
			{
				Secret: &corev1.SecretProjection{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: pgBackrestSecret.Name,
					},
				},
			},
		}
		pgDataSource.PGBackRest.Repo = crunchyv1beta1.PGBackRestRepo{
			Name: repoName,
			S3: &crunchyv1beta1.RepoS3{
				Bucket:   backupStorage.Spec.Bucket,
				Endpoint: backupStorage.Spec.EndpointURL,
				Region:   backupStorage.Spec.Region,
			},
		}
	case everestv1alpha1.BackupStorageTypeAzure:
		pgBackRestSecretIni, err := ini.Load([]byte{})
		if err != nil {
			return nil, errors.Join(err, errors.New("failed to initialize PGBackrest secret data"))
		}

		backupStorageSecret := &corev1.Secret{}
		err = r.Get(ctx, types.NamespacedName{Name: backupStorage.Spec.CredentialsSecretName, Namespace: backupStorage.Namespace}, backupStorageSecret)
		if err != nil {
			return nil, errors.Join(err, fmt.Errorf("failed to get backup storage secret %s", backupStorage.Spec.CredentialsSecretName))
		}

		err = addBackupStorageCredentialsToPGBackrestSecretIni(
			everestv1alpha1.BackupStorageTypeAzure, pgBackRestSecretIni, repoName, backupStorageSecret,
		)
		if err != nil {
			return nil, errors.Join(err, errors.New("failed to add data source storage credentials to PGBackrest secret data"))
		}
		pgBackrestSecretBuf := new(bytes.Buffer)
		if _, err = pgBackRestSecretIni.WriteTo(pgBackrestSecretBuf); err != nil {
			return nil, errors.Join(err, errors.New("failed to write PGBackrest secret data"))
		}

		secretData := pgBackrestSecretBuf.Bytes()

		pgBackrestSecret, err := r.createPGBackrestSecret(
			ctx,
			database,
			"azure.conf",
			secretData,
			database.Name+"-pgbackrest-datasource-secrets",
			nil,
		)
		if err != nil {
			return nil, errors.Join(err, errors.New("failed to create pgbackrest secret"))
		}

		pgDataSource.PGBackRest.Configuration = []corev1.VolumeProjection{
			{
				Secret: &corev1.SecretProjection{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: pgBackrestSecret.Name,
					},
				},
			},
		}
		pgDataSource.PGBackRest.Repo = crunchyv1beta1.PGBackRestRepo{
			Name: repoName,
			Azure: &crunchyv1beta1.RepoAzure{
				Container: backupStorage.Spec.Bucket,
			},
		}
	default:
		return nil, fmt.Errorf("unsupported backup storage type %s for %s", backupStorage.Spec.Type, backupStorage.Name)
	}

	return pgDataSource, nil
}

func globalDatasourceDestination(dest string, db *everestv1alpha1.DatabaseCluster, backupStorage *everestv1alpha1.BackupStorage) string {
	if dest == "" {
		dest = "/" + backupStoragePrefix(db)
	} else {
		// Extract the relevant prefix from the backup destination
		switch backupStorage.Spec.Type {
		case everestv1alpha1.BackupStorageTypeS3:
			dest = strings.TrimPrefix(dest, "s3://")
		case everestv1alpha1.BackupStorageTypeAzure:
			dest = strings.TrimPrefix(dest, "azure://")
		}

		dest = strings.TrimPrefix(dest, backupStorage.Spec.Bucket)
		dest = strings.TrimLeft(dest, "/")
		prefix := backupStoragePrefix(db)
		prefixCount := len(strings.Split(prefix, "/"))
		dest = "/" + strings.Join(strings.SplitN(dest, "/", prefixCount+1)[0:prefixCount], "/")
	}

	return dest
}

//nolint:gocognit,maintidx,gocyclo,cyclop
func (r *DatabaseClusterReconciler) reconcilePG(ctx context.Context, req ctrl.Request, database *everestv1alpha1.DatabaseCluster) error {
	version, err := r.getOperatorVersion(ctx, types.NamespacedName{
		Namespace: req.NamespacedName.Namespace,
		Name:      pgDeploymentName,
	})
	if err != nil {
		return err
	}
	clusterType, err := r.getClusterType(ctx)
	if err != nil {
		return err
	}

	pgSpec := r.defaultPGSpec()
	if clusterType == ClusterTypeEKS {
		affinity := &corev1.Affinity{
			PodAntiAffinity: &corev1.PodAntiAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
					{
						TopologyKey: "kubernetes.io/hostname",
					},
				},
			},
		}
		pgSpec.InstanceSets[0].Affinity = affinity
		pgSpec.Proxy.PGBouncer.Affinity = affinity
	}

	pg := &pgv2.PerconaPGCluster{}
	err = r.Get(ctx, types.NamespacedName{Name: database.Name, Namespace: database.Namespace}, pg)
	if err != nil && !k8serrors.IsNotFound(err) {
		return errors.Join(err, fmt.Errorf("failed to get pg object %s", database.Name))
	}
	pg.Name = database.Name
	pg.Namespace = database.Namespace
	pg.Annotations = database.Annotations
	pg.Spec = *pgSpec

	if pg.Spec.PMM == nil {
		pg.Spec.PMM = &pgv2.PMMSpec{}
	}

	if pg.Spec.PMM.Secret == "" {
		pg.Spec.PMM.Secret = fmt.Sprintf("%s%s-pmm", everestSecretsPrefix, database.Name)
	}

	if len(database.Finalizers) != 0 {
		pg.Finalizers = database.Finalizers
		database.Finalizers = []string{}
	}

	// DataSource is not needed anymore when db is ready.
	// Deleting it to prevent the future restoration conflicts.
	if database.Status.Status == everestv1alpha1.AppStateReady {
		database.Spec.DataSource = nil
	}

	if err := r.Update(ctx, database); err != nil {
		return err
	}

	engine := &everestv1alpha1.DatabaseEngine{}
	err = r.Get(ctx, types.NamespacedName{Name: pgDeploymentName, Namespace: database.Namespace}, engine)
	if err != nil {
		return errors.Join(err, fmt.Errorf("failed to get database engine %s", pgDeploymentName))
	}
	if err := controllerutil.SetControllerReference(database, pg, r.Client.Scheme()); err != nil {
		return err
	}
	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, pg, func() error {
		if !controllerutil.ContainsFinalizer(pg, finalizerDeletePGPVC) {
			controllerutil.AddFinalizer(pg, finalizerDeletePGPVC)
		}
		if !controllerutil.ContainsFinalizer(pg, finalizerDeletePGSSL) {
			controllerutil.AddFinalizer(pg, finalizerDeletePGSSL)
		}
		pg.TypeMeta = metav1.TypeMeta{
			APIVersion: pgAPIGroup + "/v2",
			Kind:       PerconaPGClusterKind,
		}

		//nolint:godox
		// FIXME add the secrets name when
		// https://jira.percona.com/browse/K8SPG-309 is fixed
		// pg.Spec.SecretsName = database.Spec.Engine.UserSecretsName
		pg.Spec.Pause = &database.Spec.Paused
		if database.Spec.Engine.Version == "" {
			database.Spec.Engine.Version = engine.BestEngineVersion()
		}
		pgEngineVersion, ok := engine.Status.AvailableVersions.Engine[database.Spec.Engine.Version]
		if !ok {
			return fmt.Errorf("engine version %s not available", database.Spec.Engine.Version)
		}
		crVersion := version.ToCRVersion()
		if pg.Spec.CRVersion != "" {
			crVersion = pg.Spec.CRVersion
		}
		pg.Spec.CRVersion = crVersion

		pg.Spec.Image = pgEngineVersion.ImagePath

		pgMajorVersionMatch := regexp.MustCompile(`^(\d+)`).FindStringSubmatch(database.Spec.Engine.Version)
		if len(pgMajorVersionMatch) < 2 {
			return fmt.Errorf("failed to extract the major version from %s", database.Spec.Engine.Version)
		}
		pgMajorVersion, err := strconv.Atoi(pgMajorVersionMatch[1])
		if err != nil {
			return err
		}
		pg.Spec.PostgresVersion = pgMajorVersion

		if err := r.updatePGConfig(pg, database); err != nil {
			return errors.Join(err, errors.New("could not update PG config"))
		}

		pg.Spec.InstanceSets[0].Replicas = &database.Spec.Engine.Replicas
		if !database.Spec.Engine.Resources.CPU.IsZero() {
			pg.Spec.InstanceSets[0].Resources.Limits[corev1.ResourceCPU] = database.Spec.Engine.Resources.CPU
		}
		if !database.Spec.Engine.Resources.Memory.IsZero() {
			pg.Spec.InstanceSets[0].Resources.Limits[corev1.ResourceMemory] = database.Spec.Engine.Resources.Memory
		}
		pg.Spec.InstanceSets[0].DataVolumeClaimSpec = corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			},
			StorageClassName: database.Spec.Engine.Storage.Class,
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: database.Spec.Engine.Storage.Size,
				},
			},
		}

		pgbouncerAvailVersions, ok := engine.Status.AvailableVersions.Proxy["pgbouncer"]
		if !ok {
			return errors.New("pgbouncer version not available")
		}

		pgbouncerVersion, ok := pgbouncerAvailVersions[database.Spec.Engine.Version]
		if !ok {
			return fmt.Errorf("pgbouncer version %s not available", database.Spec.Engine.Version)
		}

		pg.Spec.Proxy.PGBouncer.Image = pgbouncerVersion.ImagePath

		if database.Spec.Proxy.Replicas == nil {
			// By default we set the same number of replicas as the engine
			pg.Spec.Proxy.PGBouncer.Replicas = &database.Spec.Engine.Replicas
		} else {
			pg.Spec.Proxy.PGBouncer.Replicas = database.Spec.Proxy.Replicas
		}
		//nolint:godox
		// TODO add support for database.Spec.LoadBalancer.LoadBalancerSourceRanges
		// https://jira.percona.com/browse/K8SPG-311
		switch database.Spec.Proxy.Expose.Type {
		case everestv1alpha1.ExposeTypeInternal:
			pg.Spec.Proxy.PGBouncer.ServiceExpose = &pgv2.ServiceExpose{
				Type: string(corev1.ServiceTypeClusterIP),
			}
		case everestv1alpha1.ExposeTypeExternal:
			pg.Spec.Proxy.PGBouncer.ServiceExpose = &pgv2.ServiceExpose{
				Type: string(corev1.ServiceTypeLoadBalancer),
			}
		default:
			return fmt.Errorf("invalid expose type %s", database.Spec.Proxy.Expose.Type)
		}

		if !database.Spec.Proxy.Resources.CPU.IsZero() {
			pg.Spec.Proxy.PGBouncer.Resources.Limits[corev1.ResourceCPU] = database.Spec.Proxy.Resources.CPU
		}
		if !database.Spec.Proxy.Resources.Memory.IsZero() {
			pg.Spec.Proxy.PGBouncer.Resources.Limits[corev1.ResourceMemory] = database.Spec.Proxy.Resources.Memory
		}
		pg.Spec.Proxy.PGBouncer.ExposeSuperusers = true
		pg.Spec.Users = []crunchyv1beta1.PostgresUserSpec{
			{
				Name:       "postgres",
				SecretName: crunchyv1beta1.PostgresIdentifier(database.Spec.Engine.UserSecretsName),
			},
		}

		monitoring := &everestv1alpha1.MonitoringConfig{}
		if database.Spec.Monitoring != nil && database.Spec.Monitoring.MonitoringConfigName != "" {
			err := r.Get(ctx, types.NamespacedName{
				Namespace: r.monitoringNamespace,
				Name:      database.Spec.Monitoring.MonitoringConfigName,
			}, monitoring)
			if err != nil {
				return err
			}
			if !monitoring.IsNamespaceAllowed(database.Namespace) {
				return fmt.Errorf("%s namespace is not allowed to use for %s monitoring config", database.Namespace, monitoring.Name)
			}
		}

		// We have to assign the default spec here explicitly becase PG reconciliation
		// does not assign the default spec in this createOrUpdate mutate function.
		pg.Spec.PMM = pgSpec.PMM
		if monitoring.Spec.Type == everestv1alpha1.PMMMonitoringType { //nolint:dupl
			image := defaultPMMClientImage
			if monitoring.Spec.PMM.Image != "" {
				image = monitoring.Spec.PMM.Image
			}

			pg.Spec.PMM.Enabled = true
			pmmURL, err := url.Parse(monitoring.Spec.PMM.URL)
			if err != nil {
				return errors.Join(err, errors.New("invalid monitoring URL"))
			}
			pg.Spec.PMM.ServerHost = pmmURL.Hostname()
			pg.Spec.PMM.Image = image
			pg.Spec.PMM.Resources = database.Spec.Monitoring.Resources

			apiKey, err := r.getSecretFromMonitoringConfig(ctx, monitoring)
			if err != nil {
				return err
			}

			err = r.createOrUpdateSecretData(ctx, database, pg.Spec.PMM.Secret,
				map[string][]byte{
					"PMM_SERVER_KEY": []byte(apiKey),
				},
			)
			if err != nil {
				return err
			}
		}

		pg.Spec.Backups, err = r.reconcilePGBackupsSpec(ctx, pg.Spec.Backups, database, engine)
		if err != nil {
			return err
		}

		pg.Spec.DataSource = nil
		if database.Spec.DataSource != nil {
			pg.Spec.DataSource, err = r.genPGDataSourceSpec(ctx, database)
			if err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return err
	}

	database.Status.Status = everestv1alpha1.AppState(pg.Status.State)
	// If a restore is running for this database, set the database status to
	// restoring
	restoreList := &everestv1alpha1.DatabaseClusterRestoreList{}
	listOps := &client.ListOptions{
		FieldSelector: fields.OneTermEqualSelector(dbClusterRestoreDBClusterNameField, database.Name),
		Namespace:     database.Namespace,
	}
	err = r.List(ctx, restoreList, listOps)
	if err != nil {
		return errors.Join(err, errors.New("failed to list DatabaseClusterRestore objects"))
	}
	for _, restore := range restoreList.Items {
		if !restore.IsComplete(database.Spec.Engine.Type) {
			database.Status.Status = everestv1alpha1.AppStateRestoring
			break
		}
	}

	database.Status.Hostname = pg.Status.Host
	database.Status.Ready = pg.Status.Postgres.Ready + pg.Status.PGBouncer.Ready
	database.Status.Size = pg.Status.Postgres.Size + pg.Status.PGBouncer.Size
	database.Status.Port = 5432
	return r.Status().Update(ctx, database)
}

func (r *DatabaseClusterReconciler) updatePGConfig(
	pg *pgv2.PerconaPGCluster, db *everestv1alpha1.DatabaseCluster,
) error {
	//nolint:godox
	// TODO: uncomment once https://perconadev.atlassian.net/browse/K8SPG-518 done
	// if db.Spec.Engine.Config == "" {
	//	 if pg.Spec.Patroni == nil {
	//		 return nil
	//	 }
	//	 pg.Spec.Patroni.DynamicConfiguration = nil
	//	 return nil
	// }

	parser := NewPGConfigParser(db.Spec.Engine.Config)
	cfg, err := parser.ParsePGConfig()
	if err != nil {
		return err
	}

	//nolint:godox
	// TODO: remove once https://perconadev.atlassian.net/browse/K8SPG-518 done
	cfg["track_commit_timestamp"] = "on"

	if len(cfg) == 0 {
		return nil
	}

	if pg.Spec.Patroni == nil {
		pg.Spec.Patroni = &crunchyv1beta1.PatroniSpec{}
	}

	if pg.Spec.Patroni.DynamicConfiguration == nil {
		pg.Spec.Patroni.DynamicConfiguration = make(crunchyv1beta1.SchemalessObject)
	}

	dc := pg.Spec.Patroni.DynamicConfiguration
	if _, ok := dc["postgresql"]; !ok {
		dc["postgresql"] = make(map[string]any)
	}

	dcPG, ok := dc["postgresql"].(map[string]any)
	if !ok {
		return errors.New("could not assert postgresql as map[string]any")
	}

	if _, ok := dcPG["parameters"]; !ok {
		dcPG["parameters"] = make(map[string]any)
	}

	if _, ok := dcPG["parameters"].(map[string]any); !ok {
		return errors.New("could not assert postgresql.parameters as map[string]any")
	}

	dcPG["parameters"] = cfg

	return nil
}

func (r *DatabaseClusterReconciler) reconcileLabels(ctx context.Context, database *everestv1alpha1.DatabaseCluster) error {
	needsUpdate := false
	if len(database.ObjectMeta.Labels) == 0 {
		database.ObjectMeta.Labels = map[string]string{
			databaseClusterNameLabel: database.Name,
		}
		needsUpdate = true
	}
	if database.Spec.DataSource != nil {
		if _, ok := database.ObjectMeta.Labels[fmt.Sprintf(backupStorageNameLabelTmpl, database.Spec.DataSource.DBClusterBackupName)]; !ok {
			database.ObjectMeta.Labels[fmt.Sprintf(backupStorageNameLabelTmpl, database.Spec.DataSource.DBClusterBackupName)] = backupStorageLabelValue
			needsUpdate = true
		}
	}
	if database.Spec.Backup.PITR.BackupStorageName != nil {
		if _, ok := database.ObjectMeta.Labels[fmt.Sprintf(backupStorageNameLabelTmpl, *database.Spec.Backup.PITR.BackupStorageName)]; !ok {
			database.ObjectMeta.Labels[fmt.Sprintf(backupStorageNameLabelTmpl, *database.Spec.Backup.PITR.BackupStorageName)] = backupStorageLabelValue
			needsUpdate = true
		}
	}

	for _, schedule := range database.Spec.Backup.Schedules {
		if _, ok := database.ObjectMeta.Labels[fmt.Sprintf(backupStorageNameLabelTmpl, schedule.BackupStorageName)]; !ok {
			database.ObjectMeta.Labels[fmt.Sprintf(backupStorageNameLabelTmpl, schedule.BackupStorageName)] = backupStorageLabelValue
			needsUpdate = true
		}
	}
	if database.Spec.Monitoring != nil && database.Spec.Monitoring.MonitoringConfigName != "" {
		monitoringConfigName, ok := database.ObjectMeta.Labels[monitoringConfigNameLabel]
		if !ok || monitoringConfigName != database.Spec.Monitoring.MonitoringConfigName {
			database.ObjectMeta.Labels[monitoringConfigNameLabel] = database.Spec.Monitoring.MonitoringConfigName
			needsUpdate = true
		}
	}
	for key := range database.ObjectMeta.Labels {
		if key == databaseClusterNameLabel || key == monitoringConfigNameLabel {
			continue
		}
		var found bool
		for _, schedule := range database.Spec.Backup.Schedules {
			if key == fmt.Sprintf(backupStorageNameLabelTmpl, schedule.BackupStorageName) {
				found = true
				break
			}
		}
		if !found {
			delete(database.ObjectMeta.Labels, key)
			needsUpdate = true
		}
	}
	if needsUpdate {
		return r.Update(ctx, database)
	}
	return nil
}

func (r *DatabaseClusterReconciler) getOperatorVersion(ctx context.Context, name types.NamespacedName) (*Version, error) {
	unstructuredResource := &unstructured.Unstructured{}
	unstructuredResource.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "apps",
		Kind:    "Deployment",
		Version: "v1",
	})
	deployment := &appsv1.Deployment{}
	if err := r.Get(ctx, name, unstructuredResource); err != nil {
		return nil, err
	}
	err := runtime.DefaultUnstructuredConverter.
		FromUnstructured(unstructuredResource.Object, deployment)
	if err != nil {
		return nil, err
	}
	version := strings.Split(deployment.Spec.Template.Spec.Containers[0].Image, ":")[1]
	return NewVersion(version)
}

func (r *DatabaseClusterReconciler) addPXCKnownTypes(scheme *runtime.Scheme) error {
	pxcSchemeGroupVersion := schema.GroupVersion{Group: pxcAPIGroup, Version: "v1"}
	scheme.AddKnownTypes(pxcSchemeGroupVersion,
		&pxcv1.PerconaXtraDBCluster{}, &pxcv1.PerconaXtraDBClusterList{})

	metav1.AddToGroupVersion(scheme, pxcSchemeGroupVersion)
	return nil
}

func (r *DatabaseClusterReconciler) addPSMDBKnownTypes(scheme *runtime.Scheme) error {
	psmdbSchemeGroupVersion := schema.GroupVersion{Group: psmdbAPIGroup, Version: "v1"}
	scheme.AddKnownTypes(psmdbSchemeGroupVersion,
		&psmdbv1.PerconaServerMongoDB{}, &psmdbv1.PerconaServerMongoDBList{})

	metav1.AddToGroupVersion(scheme, psmdbSchemeGroupVersion)
	return nil
}

func (r *DatabaseClusterReconciler) addPGKnownTypes(scheme *runtime.Scheme) error {
	pgSchemeGroupVersion := schema.GroupVersion{Group: pgAPIGroup, Version: "v2"}
	scheme.AddKnownTypes(pgSchemeGroupVersion,
		&pgv2.PerconaPGCluster{}, &pgv2.PerconaPGClusterList{})

	metav1.AddToGroupVersion(scheme, pgSchemeGroupVersion)
	return nil
}

func (r *DatabaseClusterReconciler) addPSMDBToScheme(scheme *runtime.Scheme) error {
	builder := runtime.NewSchemeBuilder(r.addPSMDBKnownTypes)
	return builder.AddToScheme(scheme)
}

func (r *DatabaseClusterReconciler) addPXCToScheme(scheme *runtime.Scheme) error {
	builder := runtime.NewSchemeBuilder(r.addPXCKnownTypes)
	return builder.AddToScheme(scheme)
}

func (r *DatabaseClusterReconciler) addPGToScheme(scheme *runtime.Scheme) error {
	builder := runtime.NewSchemeBuilder(r.addPGKnownTypes)
	return builder.AddToScheme(scheme)
}

// SetupWithManager sets up the controller with the Manager.
func (r *DatabaseClusterReconciler) SetupWithManager(mgr ctrl.Manager, systemNamespace, monitoringNamespace string) error {
	if err := r.initIndexers(context.Background(), mgr); err != nil {
		return err
	}

	unstructuredResource := &unstructured.Unstructured{}
	unstructuredResource.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "apiextensions.k8s.io",
		Kind:    "CustomResourceDefinition",
		Version: "v1",
	})
	controller := ctrl.NewControllerManagedBy(mgr).
		For(&everestv1alpha1.DatabaseCluster{})
	err := r.Get(context.Background(), types.NamespacedName{Name: pxcCRDName}, unstructuredResource)
	if err == nil {
		if err := r.addPXCToScheme(r.Scheme); err == nil {
			controller.Owns(&pxcv1.PerconaXtraDBCluster{})
		}
	}
	err = r.Get(context.Background(), types.NamespacedName{Name: psmdbCRDName}, unstructuredResource)
	if err == nil {
		if err := r.addPSMDBToScheme(r.Scheme); err == nil {
			controller.Owns(&psmdbv1.PerconaServerMongoDB{})
		}
	}
	err = r.Get(context.Background(), types.NamespacedName{Name: pgCRDName}, unstructuredResource)
	if err == nil {
		if err := r.addPGToScheme(r.Scheme); err == nil {
			controller.Owns(&pgv2.PerconaPGCluster{})
		}
	}

	r.initWatchers(controller)
	r.systemNamespace = systemNamespace
	r.monitoringNamespace = monitoringNamespace

	return controller.Complete(r)
}

func (r *DatabaseClusterReconciler) initIndexers(ctx context.Context, mgr ctrl.Manager) error {
	// Index the BackupStorage's CredentialsSecretName field so that it can be
	// used by the databaseClustersThatReferenceSecret function to
	// find all DatabaseClusters that reference a specific secret through the
	// BackupStorage's CredentialsSecretName field
	err := mgr.GetFieldIndexer().IndexField(
		ctx, &everestv1alpha1.BackupStorage{}, credentialsSecretNameField,
		func(o client.Object) []string {
			var res []string
			backupStorage, ok := o.(*everestv1alpha1.BackupStorage)
			if !ok {
				return res
			}
			res = append(res, backupStorage.Spec.CredentialsSecretName)
			return res
		},
	)
	if err != nil {
		return err
	}

	// Index the BackupStorageName so that it can be used by the
	// databaseClustersThatReferenceObject function to find all
	// DatabaseClusters that reference a specific BackupStorage through the
	// BackupStorageName field
	err = mgr.GetFieldIndexer().IndexField(
		ctx, &everestv1alpha1.DatabaseCluster{}, backupStorageNameField,
		func(o client.Object) []string {
			var res []string
			database, ok := o.(*everestv1alpha1.DatabaseCluster)
			if !ok || !database.Spec.Backup.Enabled {
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

	// Index the credentialsSecretName field in MonitoringConfig.
	err = mgr.GetFieldIndexer().IndexField(
		ctx, &everestv1alpha1.MonitoringConfig{}, monitoringConfigSecretNameField,
		func(o client.Object) []string {
			var res []string
			mc, ok := o.(*everestv1alpha1.MonitoringConfig)
			if !ok {
				return res
			}
			res = append(res, mc.Spec.CredentialsSecretName)
			return res
		},
	)

	return err
}

func (r *DatabaseClusterReconciler) initWatchers(controller *builder.Builder) {
	// In PG reconciliation we create a backup credentials secret because the
	// PG operator requires this secret to be encoded differently from the
	// generic one used in PXC and PSMDB. Therefore, we need to watch for
	// secrets, specifically the ones that are referenced in DatabaseCluster
	// CRs, and trigger a reconciliation if these change so that we can
	// reenconde the secret required by PG.
	controller.Owns(&everestv1alpha1.BackupStorage{})
	controller.Watches(
		&everestv1alpha1.BackupStorage{},
		handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
			return r.databaseClustersThatReferenceObject(ctx, backupStorageNameField, obj)
		}),
		builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
	)

	controller.Owns(&everestv1alpha1.MonitoringConfig{})
	controller.Watches(
		&everestv1alpha1.MonitoringConfig{},
		handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
			return r.databaseClustersThatReferenceObject(ctx, monitoringConfigNameField, obj)
		}),
		builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
	)

	controller.Owns(&corev1.Secret{})
	controller.Watches(
		&corev1.Secret{},
		handler.EnqueueRequestsFromMapFunc(r.databaseClustersThatReferenceSecret),
		builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
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
		builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
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
		builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
	)
}

// databaseClustersThatReferenceObject returns a list of reconcile
// requests for all DatabaseClusters that reference the given object by the provided keyPath.
func (r *DatabaseClusterReconciler) databaseClustersThatReferenceObject(ctx context.Context, keyPath string, obj client.Object) []reconcile.Request {
	attachedDatabaseClusters := &everestv1alpha1.DatabaseClusterList{}
	listOps := &client.ListOptions{
		FieldSelector: fields.OneTermEqualSelector(keyPath, obj.GetName()),
		Namespace:     obj.GetNamespace(),
	}
	err := r.List(ctx, attachedDatabaseClusters, listOps)
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
}

// databaseClustersThatReferenceSecret returns a list of reconcile
// requests for all DatabaseClusters that reference the given secret.
func (r *DatabaseClusterReconciler) databaseClustersThatReferenceSecret(ctx context.Context, secret client.Object) []reconcile.Request {
	logger := log.FromContext(ctx)

	// BackupStorage
	var res []reconcile.Request
	bsList := &everestv1alpha1.BackupStorageList{}
	err := r.findObjectsBySecretName(ctx, secret, credentialsSecretNameField, bsList)
	if err != nil {
		logger.Error(err, "could not find BackupStorage by secret name")
	}
	if err == nil {
		var items []client.Object
		for _, i := range bsList.Items {
			i := i
			items = append(items, &i)
		}
		res = append(res, r.getDBClustersReconcileRequestsByRelatedObjectName(ctx, items, backupStorageNameField)...)
	}

	// MonitoringConfig
	mcList := &everestv1alpha1.MonitoringConfigList{}
	err = r.findObjectsBySecretName(ctx, secret, monitoringConfigSecretNameField, mcList)
	if err != nil {
		logger.Error(err, "could not find MonitoringConfig by secret name")
	}
	if err == nil {
		var items []client.Object
		for _, i := range mcList.Items {
			i := i
			items = append(items, &i)
		}
		res = append(res, r.getDBClustersReconcileRequestsByRelatedObjectName(ctx, items, monitoringConfigNameField)...)
	}

	return res
}

func (r *DatabaseClusterReconciler) findObjectsBySecretName(ctx context.Context, secret client.Object, fieldPath string, obj client.ObjectList) error {
	listOpts := &client.ListOptions{
		FieldSelector: fields.OneTermEqualSelector(fieldPath, secret.GetName()),
		Namespace:     secret.GetNamespace(),
	}
	return r.List(ctx, obj, listOpts)
}

func (r *DatabaseClusterReconciler) getDBClustersReconcileRequestsByRelatedObjectName(ctx context.Context, items []client.Object, fieldPath string) []reconcile.Request {
	var requests []reconcile.Request
	for _, i := range items {
		attachedDatabaseClusters := &everestv1alpha1.DatabaseClusterList{}
		listOps := &client.ListOptions{
			FieldSelector: fields.OneTermEqualSelector(fieldPath, i.GetName()),
			Namespace:     i.GetNamespace(),
		}
		if err := r.List(ctx, attachedDatabaseClusters, listOps); err != nil {
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

func mergeMapInternal(dst map[string]interface{}, src map[string]interface{}, parent string) error {
	for k, v := range src {
		if dst[k] != nil && reflect.TypeOf(dst[k]) != reflect.TypeOf(v) {
			return fmt.Errorf("type mismatch for %s.%s, %T != %T", parent, k, dst[k], v)
		}
		switch v.(type) { //nolint:gocritic
		case map[string]interface{}:
			switch dst[k].(type) { //nolint:gocritic
			case nil:
				dst[k] = v
			case map[string]interface{}: //nolint:forcetypeassert
				err := mergeMapInternal(dst[k].(map[string]interface{}),
					v.(map[string]interface{}), fmt.Sprintf("%s.%s", parent, k))
				if err != nil {
					return err
				}
			default:
				return fmt.Errorf("type mismatch for %s.%s, %T != %T", parent, k, dst[k], v)
			}
		default:
			dst[k] = v
		}
	}
	return nil
}

func mergeMap(dst map[string]interface{}, src map[string]interface{}) error {
	return mergeMapInternal(dst, src, "")
}

func (r *DatabaseClusterReconciler) applyTemplate(
	ctx context.Context,
	obj interface{},
	kind string,
	namespacedName types.NamespacedName,
) error {
	unstructuredTemplate := &unstructured.Unstructured{}
	unstructuredDB := &unstructured.Unstructured{}

	unstructuredTemplate.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "everest.percona.com",
		Kind:    kind,
		Version: "v1alpha1",
	})
	err := r.Get(ctx, namespacedName, unstructuredTemplate)
	if err != nil {
		return err
	}

	unstructuredDB.Object, err = runtime.DefaultUnstructuredConverter.
		ToUnstructured(obj)
	if err != nil {
		return err
	}
	//nolint:forcetypeassert
	err = mergeMap(unstructuredDB.Object["spec"].(map[string]interface{}),
		unstructuredTemplate.Object["spec"].(map[string]interface{}))
	if err != nil {
		return err
	}

	if unstructuredTemplate.Object["metadata"].(map[string]interface{})["annotations"] != nil { //nolint:forcetypeassert
		if unstructuredDB.Object["metadata"].(map[string]interface{})["annotations"] == nil { //nolint:forcetypeassert
			unstructuredDB.Object["metadata"].(map[string]interface{})["annotations"] = make(map[string]interface{}) //nolint:forcetypeassert
		}
		//nolint:forcetypeassert
		err = mergeMap(unstructuredDB.Object["metadata"].(map[string]interface{})["annotations"].(map[string]interface{}),
			unstructuredTemplate.Object["metadata"].(map[string]interface{})["annotations"].(map[string]interface{}))
		if err != nil {
			return err
		}
	}

	if unstructuredTemplate.Object["metadata"].(map[string]interface{})["labels"] != nil { //nolint:forcetypeassert
		if unstructuredDB.Object["metadata"].(map[string]interface{})["labels"] == nil { //nolint:forcetypeassert
			unstructuredDB.Object["metadata"].(map[string]interface{})["labels"] = make(map[string]interface{}) //nolint:forcetypeassert
		}
		//nolint:forcetypeassert
		err = mergeMap(unstructuredDB.Object["metadata"].(map[string]interface{})["labels"].(map[string]interface{}),
			unstructuredTemplate.Object["metadata"].(map[string]interface{})["labels"].(map[string]interface{}))
		if err != nil {
			return err
		}
	}

	err = runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredDB.Object, obj)
	if err != nil {
		return err
	}
	return nil
}

func getObjectHash(obj runtime.Object) (string, error) {
	var dataToMarshall interface{}
	switch object := obj.(type) {
	case *appsv1.StatefulSet:
		dataToMarshall = object.Spec
	case *appsv1.Deployment:
		dataToMarshall = object.Spec
	case *corev1.Service:
		dataToMarshall = object.Spec
	default:
		dataToMarshall = obj
	}
	data, err := json.Marshal(dataToMarshall)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

func isObjectMetaEqual(oldObj, newObj metav1.Object) bool {
	return reflect.DeepEqual(oldObj.GetAnnotations(), newObj.GetAnnotations()) &&
		reflect.DeepEqual(oldObj.GetLabels(), newObj.GetLabels())
}

// createOrUpdate creates or updates a resource.
// With patchSecretData the new secret Data is applied on top of the original secret's Data.
func (r *DatabaseClusterReconciler) createOrUpdate(ctx context.Context, obj client.Object, patchSecretData bool) error {
	hash, err := getObjectHash(obj)
	if err != nil {
		return errors.Join(err, errors.New("calculate object hash"))
	}

	val := reflect.ValueOf(obj)
	if val.Kind() == reflect.Ptr {
		val = reflect.Indirect(val)
	}
	oldObject, ok := reflect.New(val.Type()).Interface().(client.Object)
	if !ok {
		return errors.New("failed type conversion")
	}

	if obj.GetName() == "" && obj.GetGenerateName() != "" {
		return r.Create(ctx, obj)
	}

	err = r.Get(ctx, types.NamespacedName{
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
	}, oldObject)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return r.Create(ctx, obj)
		}
		return errors.Join(err, errors.New("get object"))
	}

	oldHash, err := getObjectHash(oldObject)
	if err != nil {
		return errors.Join(err, errors.New("calculate old object hash"))
	}

	if oldHash != hash || !isObjectMetaEqual(obj, oldObject) {
		return r.updateObject(ctx, obj, oldObject, patchSecretData)
	}

	return nil
}

func (r *DatabaseClusterReconciler) updateObject(ctx context.Context, obj, oldObject client.Object, patchSecretData bool) error {
	obj.SetResourceVersion(oldObject.GetResourceVersion())
	switch object := obj.(type) {
	case *corev1.Service:
		oldObjectService, ok := oldObject.(*corev1.Service)
		if !ok {
			return errors.New("failed type conversion to service")
		}
		object.Spec.ClusterIP = oldObjectService.Spec.ClusterIP
		if object.Spec.Type == corev1.ServiceTypeLoadBalancer {
			object.Spec.HealthCheckNodePort = oldObjectService.Spec.HealthCheckNodePort
		}
	case *corev1.Secret:
		if patchSecretData {
			s, ok := oldObject.(*corev1.Secret)
			if !ok {
				return errors.New("failed type conversion to secret")
			}
			for k, v := range object.Data {
				s.Data[k] = v
			}
			object.Data = s.Data
		}
	default:
	}

	return r.Update(ctx, obj)
}

func getPGRetentionCopies(retentionCopies int32) int {
	copies := int(retentionCopies)
	if copies == 0 {
		// Zero copies means unlimited. PGBackRest supports values 1-9999999
		copies = 9999999
	}

	return copies
}

func backupStoragePrefix(db *everestv1alpha1.DatabaseCluster) string {
	return fmt.Sprintf("%s/%s", db.Name, db.UID)
}

func (r *DatabaseClusterReconciler) defaultPGSpec() *pgv2.PerconaPGClusterSpec {
	return &pgv2.PerconaPGClusterSpec{
		InstanceSets: pgv2.PGInstanceSets{
			{
				Name: "instance1",
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{},
				},
			},
		},
		PMM: &pgv2.PMMSpec{
			Enabled: false,
			Resources: corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("300M"),
					corev1.ResourceCPU:    resource.MustParse("500m"),
				},
			},
		},
		Proxy: &pgv2.PGProxySpec{
			PGBouncer: &pgv2.PGBouncerSpec{
				Resources: corev1.ResourceRequirements{
					// XXX: Remove this once templates will be available
					Limits: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("128Mi"),
						corev1.ResourceCPU:    resource.MustParse("200m"),
					},
				},
			},
		},
	}
}

func (r *DatabaseClusterReconciler) defaultPXCSpec() *pxcv1.PerconaXtraDBClusterSpec {
	return &pxcv1.PerconaXtraDBClusterSpec{
		UpdateStrategy: pxcv1.SmartUpdateStatefulSetStrategyType,
		UpgradeOptions: pxcv1.UpgradeOptions{
			Apply:    "never",
			Schedule: "0 4 * * *",
		},
		PXC: &pxcv1.PXCSpec{
			PodSpec: &pxcv1.PodSpec{
				ServiceType: corev1.ServiceTypeClusterIP,
				Affinity: &pxcv1.PodAffinity{
					TopologyKey: pointer.ToString(pxcv1.AffinityTopologyKeyOff),
				},
				PodDisruptionBudget: &pxcv1.PodDisruptionBudgetSpec{
					MaxUnavailable: &maxUnavailable,
				},
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("1G"),
						corev1.ResourceCPU:    resource.MustParse("600m"),
					},
				},
			},
		},
		PMM: &pxcv1.PMMSpec{
			Enabled: false,
			Resources: corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("300M"),
					corev1.ResourceCPU:    resource.MustParse("500m"),
				},
			},
		},
		HAProxy: &pxcv1.HAProxySpec{
			PodSpec: pxcv1.PodSpec{
				Enabled: false,
				Affinity: &pxcv1.PodAffinity{
					TopologyKey: pointer.ToString(pxcv1.AffinityTopologyKeyOff),
				},
				Resources: corev1.ResourceRequirements{
					// XXX: Remove this once templates will be available
					Limits: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("1G"),
						corev1.ResourceCPU:    resource.MustParse("600m"),
					},
				},
				ReadinessProbes: corev1.Probe{TimeoutSeconds: 30},
				LivenessProbes:  corev1.Probe{TimeoutSeconds: 30},
			},
		},
		ProxySQL: &pxcv1.PodSpec{
			Enabled: false,
			Affinity: &pxcv1.PodAffinity{
				TopologyKey: pointer.ToString(pxcv1.AffinityTopologyKeyOff),
			},
			Resources: corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("1G"),
					corev1.ResourceCPU:    resource.MustParse("600m"),
				},
			},
		},
	}
}

func (r *DatabaseClusterReconciler) defaultPSMDBSpec() *psmdbv1.PerconaServerMongoDBSpec {
	return &psmdbv1.PerconaServerMongoDBSpec{
		UpdateStrategy: psmdbv1.SmartUpdateStatefulSetStrategyType,
		UpgradeOptions: psmdbv1.UpgradeOptions{
			Apply:    "disabled",
			Schedule: "0 4 * * *",
		},
		PMM: psmdbv1.PMMSpec{
			Enabled: false,
			Resources: corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("300M"),
					corev1.ResourceCPU:    resource.MustParse("500m"),
				},
			},
		},
		Replsets: []*psmdbv1.ReplsetSpec{
			{
				Name: "rs0",
				MultiAZ: psmdbv1.MultiAZ{
					PodDisruptionBudget: &psmdbv1.PodDisruptionBudgetSpec{
						MaxUnavailable: &maxUnavailable,
					},
					Affinity: &psmdbv1.PodAffinity{
						TopologyKey: pointer.ToString(psmdbv1.AffinityOff),
					},
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{},
					},
				},
			},
		},
		Sharding: psmdbv1.Sharding{
			Enabled: false,
		},
	}
}

func (r *DatabaseClusterReconciler) reconcileBackupStorageSecret(
	ctx context.Context,
	backupStorage *everestv1alpha1.BackupStorage,
	database *everestv1alpha1.DatabaseCluster,
) error {
	secret := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Name: backupStorage.Spec.CredentialsSecretName, Namespace: database.Namespace}, secret)
	if err != nil && !k8serrors.IsNotFound(err) {
		return err
	}
	if secret.Name != "" {
		return nil
	}

	err = r.Get(ctx, types.NamespacedName{Name: backupStorage.Spec.CredentialsSecretName, Namespace: r.systemNamespace}, secret)
	if err != nil {
		return err
	}
	secret.ObjectMeta = metav1.ObjectMeta{
		Namespace: database.Namespace,
		Name:      backupStorage.Spec.CredentialsSecretName,
		Labels: map[string]string{
			labelBackupStorageName: backupStorage.Name,
		},
	}
	if !backupStorage.IsNamespaceAllowed(database.Namespace) {
		return fmt.Errorf("%s namespace is not allowed to use for %s backup storage", database.Namespace, backupStorage.Name)
	}
	return r.Create(ctx, secret)
}

func getActiveStorage(psmdb *psmdbv1.PerconaServerMongoDB) string {
	for name := range psmdb.Spec.Backup.Storages {
		return name
	}
	return ""
}
