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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/AlekSi/pointer"
	goversion "github.com/hashicorp/go-version"
	pgv2 "github.com/percona/percona-postgresql-operator/pkg/apis/pgv2.percona.com/v2"
	crunchyv1beta1 "github.com/percona/percona-postgresql-operator/pkg/apis/postgres-operator.crunchydata.com/v1beta1"
	psmdbv1 "github.com/percona/percona-server-mongodb-operator/pkg/apis/psmdb/v1"
	pxcv1 "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/pkg/errors"
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

	pxcDeploymentName   = "percona-xtradb-cluster-operator"
	psmdbDeploymentName = "percona-server-mongodb-operator"
	pgDeploymentName    = "percona-postgresql-operator"

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

	memorySmallSize  = int64(2) * 1000 * 1000 * 1000
	memoryMediumSize = int64(8) * 1000 * 1000 * 1000
	memoryLargeSize  = int64(32) * 1000 * 1000 * 1000

	pxcDefaultConfigurationTemplate = `[mysqld]
wsrep_provider_options="gcache.size=%s"
wsrep_trx_fragment_unit='bytes'
wsrep_trx_fragment_size=3670016
`
	pxcMinimalConfigurationTemplate = `[mysqld]
wsrep_provider_options="gcache.size=%s"
`
	haProxyDefaultConfigurationTemplate = `timeout client 28800s
timeout connect 100500
timeout server 28800s
`
	psmdbDefaultConfigurationTemplate = `
      operationProfiling:
        mode: slowOp
`
	objectStorageNameField          = ".spec.backup.schedules.objectStorageName"
	credentialsSecretNameField      = ".spec.credentialsSecretName" //nolint:gosec
	monitoringConfigNameField       = ".spec.monitoring.monitoringConfigName"
	monitoringConfigSecretNameField = ".spec.credentialsSecretName" //nolint:gosec
)

var operatorDeployment = map[everestv1alpha1.EngineType]string{
	everestv1alpha1.DatabaseEnginePXC:        pxcDeploymentName,
	everestv1alpha1.DatabaseEnginePSMDB:      psmdbDeploymentName,
	everestv1alpha1.DatabaseEnginePostgresql: pgDeploymentName,
}

var defaultPXCSpec = pxcv1.PerconaXtraDBClusterSpec{
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
				Limits: corev1.ResourceList{},
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
				Limits: corev1.ResourceList{},
			},
		},
	},
	ProxySQL: &pxcv1.PodSpec{
		Enabled: false,
		Affinity: &pxcv1.PodAffinity{
			TopologyKey: pointer.ToString(pxcv1.AffinityTopologyKeyOff),
		},
		Resources: corev1.ResourceRequirements{
			Limits: corev1.ResourceList{},
		},
	},
}

var (
	maxUnavailable   = intstr.FromInt(1)
	defaultPSMDBSpec = psmdbv1.PerconaServerMongoDBSpec{
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
		Mongod: &psmdbv1.MongodSpec{
			Net: &psmdbv1.MongodSpecNet{
				Port: 27017,
			},
			OperationProfiling: &psmdbv1.MongodSpecOperationProfiling{
				Mode:              psmdbv1.OperationProfilingModeSlowOp,
				SlowOpThresholdMs: 100,
				RateLimit:         100,
			},
			Security: &psmdbv1.MongodSpecSecurity{
				RedactClientLogData:  false,
				EnableEncryption:     pointer.ToBool(true),
				EncryptionCipherMode: psmdbv1.MongodChiperModeCBC,
			},
			SetParameter: &psmdbv1.MongodSpecSetParameter{
				TTLMonitorSleepSecs: 60,
			},
			Storage: &psmdbv1.MongodSpecStorage{
				Engine: psmdbv1.StorageEngineWiredTiger,
				MMAPv1: &psmdbv1.MongodSpecMMAPv1{
					NsSize:     16,
					Smallfiles: false,
				},
				WiredTiger: &psmdbv1.MongodSpecWiredTiger{
					CollectionConfig: &psmdbv1.MongodSpecWiredTigerCollectionConfig{
						BlockCompressor: &psmdbv1.WiredTigerCompressorSnappy,
					},
					EngineConfig: &psmdbv1.MongodSpecWiredTigerEngineConfig{
						DirectoryForIndexes: false,
						JournalCompressor:   &psmdbv1.WiredTigerCompressorSnappy,
					},
					IndexConfig: &psmdbv1.MongodSpecWiredTigerIndexConfig{
						PrefixCompression: true,
					},
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
			Enabled: true,
			ConfigsvrReplSet: &psmdbv1.ReplsetSpec{
				MultiAZ: psmdbv1.MultiAZ{
					Affinity: &psmdbv1.PodAffinity{
						TopologyKey: pointer.ToString(psmdbv1.AffinityOff),
					},
				},
				Arbiter: psmdbv1.Arbiter{
					Enabled: false,
					MultiAZ: psmdbv1.MultiAZ{
						Affinity: &psmdbv1.PodAffinity{
							TopologyKey: pointer.ToString(psmdbv1.AffinityOff),
						},
					},
				},
			},
			Mongos: &psmdbv1.MongosSpec{
				MultiAZ: psmdbv1.MultiAZ{
					Affinity: &psmdbv1.PodAffinity{
						TopologyKey: pointer.ToString(psmdbv1.AffinityOff),
					},
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{},
					},
				},
			},
		},
	}
)

var defaultPGSpec = pgv2.PerconaPGClusterSpec{
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
				Limits: corev1.ResourceList{},
			},
		},
	},
}

// DatabaseClusterReconciler reconciles a DatabaseCluster object.
type DatabaseClusterReconciler struct {
	client.Client
	Scheme *runtime.Scheme
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
//+kubebuilder:rbac:groups=everest.percona.com,resources=objectstorages,verbs=get;list;watch;create;update;patch;delete

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
	_, ok := database.ObjectMeta.Annotations[restartAnnotationKey]

	if ok && !database.Spec.Paused {
		database.Spec.Paused = true
	}
	if ok && database.Status.Status == everestv1alpha1.AppStatePaused {
		database.Spec.Paused = false
		delete(database.ObjectMeta.Annotations, restartAnnotationKey)
		if err := r.Update(ctx, database); err != nil {
			return reconcile.Result{}, err
		}
	}
	if database.Spec.Engine.UserSecretsName == "" {
		database.Spec.Engine.UserSecretsName = fmt.Sprintf("everest-secrets-%s", database.Name)
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
	return reconcile.Result{RequeueAfter: 5 * time.Second}, nil
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
	dbRestore := &everestv1alpha1.DatabaseClusterRestore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      database.Name + "-datasource",
			Namespace: database.Namespace,
		},
	}
	if err := controllerutil.SetControllerReference(database, dbRestore, r.Client.Scheme()); err != nil {
		return err
	}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, dbRestore, func() error {
		objectStorage := &everestv1alpha1.ObjectStorage{}
		err := r.Get(ctx, types.NamespacedName{Name: database.Spec.DataSource.ObjectStorageName, Namespace: database.Namespace}, objectStorage)
		if err != nil {
			return errors.Wrapf(err, "failed to get object storage %s", database.Spec.DataSource.ObjectStorageName)
		}

		dbRestore.Spec.DatabaseCluster = database.Name
		dbRestore.Spec.DatabaseType = database.Spec.Engine.Type
		dbRestore.Spec.BackupSource = &everestv1alpha1.BackupSource{
			Destination: fmt.Sprintf("s3://%s/%s", objectStorage.Spec.Bucket, database.Spec.DataSource.BackupName),
			StorageName: database.Spec.DataSource.ObjectStorageName,
			StorageType: everestv1alpha1.BackupStorageType(objectStorage.Spec.Type),
		}
		switch objectStorage.Spec.Type {
		case everestv1alpha1.ObjectStorageTypeS3:
			dbRestore.Spec.BackupSource.S3 = &everestv1alpha1.BackupStorageProviderSpec{
				Bucket:            objectStorage.Spec.Bucket,
				CredentialsSecret: objectStorage.Spec.CredentialsSecretName,
				Region:            objectStorage.Spec.Region,
				EndpointURL:       objectStorage.Spec.EndpointURL,
			}
		default:
			return errors.Errorf("unsupported object storage type %s for %s", objectStorage.Spec.Type, objectStorage.Name)
		}
		return nil
	})

	return err
}

func (r *DatabaseClusterReconciler) genPSMDBBackupSpec(
	ctx context.Context,
	database *everestv1alpha1.DatabaseCluster,
	engine *everestv1alpha1.DatabaseEngine,
) (psmdbv1.BackupSpec, error) {
	bestBackupVersion := engine.BestBackupVersion(database.Spec.Engine.Version)
	backupVersion, ok := engine.Status.AvailableVersions.Backup[bestBackupVersion]
	if !ok {
		return psmdbv1.BackupSpec{Enabled: false}, errors.Errorf("backup version %s not available", bestBackupVersion)
	}

	psmdbBackupSpec := psmdbv1.BackupSpec{
		Enabled: true,
		Image:   backupVersion.ImagePath,
	}
	storages := make(map[string]psmdbv1.BackupStorageSpec)
	var tasks []psmdbv1.BackupTaskSpec //nolint:prealloc
	for _, schedule := range database.Spec.Backup.Schedules {
		if !schedule.Enabled {
			continue
		}

		objectStorage := &everestv1alpha1.ObjectStorage{}
		err := r.Get(ctx, types.NamespacedName{Name: schedule.ObjectStorageName, Namespace: database.Namespace}, objectStorage)
		if err != nil {
			return psmdbv1.BackupSpec{Enabled: false}, errors.Wrapf(err, "failed to get object storage %s", schedule.ObjectStorageName)
		}

		switch objectStorage.Spec.Type {
		case everestv1alpha1.ObjectStorageTypeS3:
			storages[schedule.ObjectStorageName] = psmdbv1.BackupStorageSpec{
				Type: psmdbv1.BackupStorageType(objectStorage.Spec.Type),
				S3: psmdbv1.BackupStorageS3Spec{
					Bucket:            objectStorage.Spec.Bucket,
					CredentialsSecret: objectStorage.Spec.CredentialsSecretName,
					Region:            objectStorage.Spec.Region,
					EndpointURL:       objectStorage.Spec.EndpointURL,
				},
			}
		default:
			return psmdbv1.BackupSpec{Enabled: false}, errors.Errorf("unsupported object storage type %s for %s", objectStorage.Spec.Type, objectStorage.Name)
		}

		tasks = append(tasks, psmdbv1.BackupTaskSpec{
			Name:        schedule.Name,
			Enabled:     true,
			Schedule:    schedule.Schedule,
			Keep:        int(schedule.RetentionCopies),
			StorageName: schedule.ObjectStorageName,
		})
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

	psmdb := &psmdbv1.PerconaServerMongoDB{
		ObjectMeta: metav1.ObjectMeta{
			Name:        database.Name,
			Namespace:   database.Namespace,
			Annotations: database.Annotations,
		},
		Spec: defaultPSMDBSpec,
	}
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
		return errors.Wrapf(err, "failed to get database engine %s", operatorDeployment[database.Spec.Engine.Type])
	}

	if err := controllerutil.SetControllerReference(database, psmdb, r.Client.Scheme()); err != nil {
		return err
	}
	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, psmdb, func() error {
		psmdb.TypeMeta = metav1.TypeMeta{
			APIVersion: version.ToAPIVersion(psmdbAPIGroup),
			Kind:       PerconaServerMongoDBKind,
		}

		clusterType, err := r.getClusterType(ctx)
		if err != nil {
			return err
		}

		psmdb.Spec = defaultPSMDBSpec
		if clusterType == ClusterTypeEKS {
			affinity := &psmdbv1.PodAffinity{
				TopologyKey: pointer.ToString("kubernetes.io/hostname"),
			}
			psmdb.Spec.Replsets[0].MultiAZ.Affinity = affinity
			psmdb.Spec.Sharding.ConfigsvrReplSet.MultiAZ.Affinity = affinity
			psmdb.Spec.Sharding.ConfigsvrReplSet.Arbiter.MultiAZ.Affinity = affinity
			psmdb.Spec.Sharding.Mongos.MultiAZ.Affinity = affinity
		}

		dbTemplateKind, hasTemplateKind := database.ObjectMeta.Annotations[dbTemplateKindAnnotationKey]
		dbTemplateName, hasTemplateName := database.ObjectMeta.Annotations[dbTemplateNameAnnotationKey]
		if hasTemplateKind && !hasTemplateName {
			return errors.Errorf("missing %s annotation", dbTemplateNameAnnotationKey)
		}
		if !hasTemplateKind && hasTemplateName {
			return errors.Errorf("missing %s annotation", dbTemplateKindAnnotationKey)
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

		psmdb.Spec.CRVersion = version.ToCRVersion()
		psmdb.Spec.UnsafeConf = database.Spec.Engine.Replicas == 1
		psmdb.Spec.Pause = database.Spec.Paused

		if database.Spec.Engine.Version == "" {
			database.Spec.Engine.Version = engine.BestEngineVersion()
		}
		engineVersion, ok := engine.Status.AvailableVersions.Engine[database.Spec.Engine.Version]
		if !ok {
			return errors.Errorf("engine version %s not available", database.Spec.Engine.Version)
		}

		psmdb.Spec.Image = engineVersion.ImagePath

		psmdb.Spec.Secrets = &psmdbv1.SecretsSpec{
			Users: database.Spec.Engine.UserSecretsName,
		}
		psmdb.Spec.Mongod.Security.EncryptionKeySecret = fmt.Sprintf("%s-mongodb-encryption-key", database.Name)

		if database.Spec.Engine.Config != "" {
			psmdb.Spec.Replsets[0].Configuration = psmdbv1.MongoConfiguration(database.Spec.Engine.Config)
		}
		if psmdb.Spec.Replsets[0].Configuration == "" {
			// Config missing from the DatabaseCluster CR and the template (if any), apply the default one
			psmdb.Spec.Replsets[0].Configuration = psmdbv1.MongoConfiguration(psmdbDefaultConfigurationTemplate)
		}

		if database.Spec.Engine.Replicas == 0 {
			database.Spec.Engine.Replicas = 3
		}
		psmdb.Spec.Replsets[0].Size = database.Spec.Engine.Replicas
		psmdb.Spec.Replsets[0].VolumeSpec = &psmdbv1.VolumeSpec{
			PersistentVolumeClaim: psmdbv1.PVCSpec{
				PersistentVolumeClaimSpec: &corev1.PersistentVolumeClaimSpec{
					StorageClassName: database.Spec.Engine.Storage.Class,
					Resources: corev1.ResourceRequirements{
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
		psmdb.Spec.Sharding.ConfigsvrReplSet.Size = database.Spec.Engine.Replicas
		psmdb.Spec.Sharding.ConfigsvrReplSet.VolumeSpec = &psmdbv1.VolumeSpec{
			PersistentVolumeClaim: psmdbv1.PVCSpec{
				PersistentVolumeClaimSpec: &corev1.PersistentVolumeClaimSpec{
					StorageClassName: database.Spec.Engine.Storage.Class,
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: database.Spec.Engine.Storage.Size,
						},
					},
				},
			},
		}
		if database.Spec.Proxy.Replicas == nil {
			// By default we set the same number of replicas as the engine
			psmdb.Spec.Sharding.Mongos.Size = database.Spec.Engine.Replicas
		} else {
			psmdb.Spec.Sharding.Mongos.Size = *database.Spec.Proxy.Replicas
		}
		switch database.Spec.Proxy.Expose.Type {
		case everestv1alpha1.ExposeTypeInternal:
			psmdb.Spec.Sharding.Mongos.Expose = psmdbv1.MongosExpose{
				Expose: psmdbv1.Expose{
					ExposeType: corev1.ServiceTypeClusterIP,
				},
			}
		case everestv1alpha1.ExposeTypeExternal:
			psmdb.Spec.Sharding.Mongos.Expose = psmdbv1.MongosExpose{
				Expose: psmdbv1.Expose{
					ExposeType:               corev1.ServiceTypeLoadBalancer,
					LoadBalancerSourceRanges: database.Spec.Proxy.Expose.IPSourceRanges,
				},
			}
		default:
			return errors.Errorf("invalid expose type %s", database.Spec.Proxy.Expose.Type)
		}

		psmdb.Spec.Sharding.Mongos.Configuration = psmdbv1.MongoConfiguration(database.Spec.Proxy.Config)
		if !database.Spec.Proxy.Resources.CPU.IsZero() {
			psmdb.Spec.Sharding.Mongos.MultiAZ.Resources.Limits[corev1.ResourceCPU] = database.Spec.Proxy.Resources.CPU
		}
		if !database.Spec.Proxy.Resources.Memory.IsZero() {
			psmdb.Spec.Sharding.Mongos.MultiAZ.Resources.Limits[corev1.ResourceMemory] = database.Spec.Proxy.Resources.Memory
		}
		if database.Spec.Engine.Replicas == 1 {
			psmdb.Spec.Sharding.Enabled = false
			psmdb.Spec.Replsets[0].Expose.Enabled = true
			switch database.Spec.Proxy.Expose.Type {
			case everestv1alpha1.ExposeTypeInternal:
				psmdb.Spec.Replsets[0].Expose.ExposeType = corev1.ServiceTypeClusterIP
			case everestv1alpha1.ExposeTypeExternal:
				psmdb.Spec.Replsets[0].Expose.ExposeType = corev1.ServiceTypeLoadBalancer
			default:
				return errors.Errorf("invalid expose type %s", database.Spec.Proxy.Expose.Type)
			}
			psmdb.Spec.Replsets[0].Expose.ExposeType = corev1.ServiceTypeClusterIP
			psmdb.Spec.Sharding.Mongos.Expose.ExposeType = corev1.ServiceTypeClusterIP
		}

		monitoring := &everestv1alpha1.MonitoringConfig{}
		if database.Spec.Monitoring != nil && database.Spec.Monitoring.MonitoringConfigName != "" {
			err := r.Get(ctx, types.NamespacedName{
				Namespace: req.NamespacedName.Namespace,
				Name:      database.Spec.Monitoring.MonitoringConfigName,
			}, monitoring)
			if err != nil {
				return err
			}
		}

		if monitoring.Spec.Type == everestv1alpha1.PMM {
			psmdb.Spec.PMM.Enabled = true
			psmdb.Spec.PMM.ServerHost = monitoring.Spec.PMM.URL
			psmdb.Spec.PMM.Image = monitoring.Spec.PMM.Image
			psmdb.Spec.PMM.Resources = database.Spec.Monitoring.Resources

			apiKey, err := r.getSecretFromMonitoringConfig(ctx, database, monitoring, "apiKey")
			if err != nil {
				return err
			}

			err = r.createOrUpdateSecret(ctx, database, psmdb.Spec.Secrets.Users, "", map[string][]byte{
				"PMM_SERVER_API_KEY": []byte(apiKey),
			})
			if err != nil {
				return err
			}
		}

		if database.Spec.Backup.Enabled {
			psmdb.Spec.Backup, err = r.genPSMDBBackupSpec(ctx, database, engine)
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	if database.Spec.DataSource != nil {
		err = r.reconcileDBRestoreFromDataSource(ctx, database)
		if err != nil {
			return err
		}
	}

	database.Status.Hostname = psmdb.Status.Host
	database.Status.Ready = psmdb.Status.Ready
	database.Status.Size = psmdb.Status.Size
	database.Status.Status = everestv1alpha1.AppState(psmdb.Status.State)
	message := psmdb.Status.Message
	conditions := psmdb.Status.Conditions
	if message == "" && len(conditions) != 0 {
		message = conditions[len(conditions)-1].Message
	}
	database.Status.Message = message
	return r.Status().Update(ctx, database)
}

// getSecretFromMonitoringConfig retrieves the credentials secret from
// the provided MonitoringConfig by secretKey field name.
func (r *DatabaseClusterReconciler) getSecretFromMonitoringConfig(
	ctx context.Context, database *everestv1alpha1.DatabaseCluster,
	monitoring *everestv1alpha1.MonitoringConfig, secretKey string,
) (string, error) {
	var secret *corev1.Secret
	secretData := ""

	if monitoring.Spec.CredentialsSecretName != "" {
		secret = &corev1.Secret{}
		err := r.Get(ctx, types.NamespacedName{
			Name:      monitoring.Spec.CredentialsSecretName,
			Namespace: database.Namespace,
		}, secret)
		if err != nil {
			return "", err
		}

		if key, ok := secret.Data[secretKey]; ok {
			secretData = string(key)
		}
	}

	return secretData, nil
}

// createOrUpdateSecret creates or updates a secret by its name.
// If a secret is created and secretName is empty, generateName is used
// to generate a unique name.
func (r *DatabaseClusterReconciler) createOrUpdateSecret(
	ctx context.Context, database *everestv1alpha1.DatabaseCluster,
	secretName, generateName string, data map[string][]byte,
) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: database.Namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: data,
	}

	if secretName == "" {
		secret.ObjectMeta.GenerateName = generateName
	}

	return r.createOrUpdate(ctx, secret)
}

func (r *DatabaseClusterReconciler) genPXCHAProxySpec(database *everestv1alpha1.DatabaseCluster, engine *everestv1alpha1.DatabaseEngine) (*pxcv1.HAProxySpec, error) {
	haProxy := defaultPXCSpec.HAProxy

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
		haProxy.PodSpec.LoadBalancerSourceRanges = database.Spec.Proxy.Expose.IPSourceRanges
	default:
		return nil, errors.Errorf("invalid expose type %s", database.Spec.Proxy.Expose.Type)
	}

	haProxy.PodSpec.Configuration = database.Spec.Proxy.Config

	haProxyAvailVersions, ok := engine.Status.AvailableVersions.Proxy[everestv1alpha1.ProxyTypeHAProxy]
	if !ok {
		return nil, errors.Errorf("haproxy version not available")
	}

	bestHAProxyVersion := haProxyAvailVersions.BestVersion()
	haProxyVersion, ok := haProxyAvailVersions[bestHAProxyVersion]
	if !ok {
		return nil, errors.Errorf("haproxy version %s not available", bestHAProxyVersion)
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
	proxySQL := defaultPXCSpec.ProxySQL

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
		proxySQL.LoadBalancerSourceRanges = database.Spec.Proxy.Expose.IPSourceRanges
	default:
		return nil, errors.Errorf("invalid expose type %s", database.Spec.Proxy.Expose.Type)
	}

	proxySQL.Configuration = database.Spec.Proxy.Config

	proxySQLAvailVersions, ok := engine.Status.AvailableVersions.Proxy[everestv1alpha1.ProxyTypeProxySQL]
	if !ok {
		return nil, errors.Errorf("proxysql version not available")
	}

	bestProxySQLVersion := proxySQLAvailVersions.BestVersion()
	proxySQLVersion, ok := proxySQLAvailVersions[bestProxySQLVersion]
	if !ok {
		return nil, errors.Errorf("proxysql version %s not available", bestProxySQLVersion)
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

func (r *DatabaseClusterReconciler) genPXCBackupSpec(
	ctx context.Context,
	database *everestv1alpha1.DatabaseCluster,
	engine *everestv1alpha1.DatabaseEngine,
) (*pxcv1.PXCScheduledBackup, error) {
	bestBackupVersion := engine.BestBackupVersion(database.Spec.Engine.Version)
	backupVersion, ok := engine.Status.AvailableVersions.Backup[bestBackupVersion]
	if !ok {
		return nil, errors.Errorf("backup version %s not available", bestBackupVersion)
	}

	pxcBackupSpec := &pxcv1.PXCScheduledBackup{
		Image: backupVersion.ImagePath,
	}

	storages := make(map[string]*pxcv1.BackupStorageSpec)
	var pxcSchedules []pxcv1.PXCScheduledBackupSchedule //nolint:prealloc
	for _, schedule := range database.Spec.Backup.Schedules {
		if !schedule.Enabled {
			continue
		}

		objectStorage := &everestv1alpha1.ObjectStorage{}
		err := r.Get(ctx, types.NamespacedName{Name: schedule.ObjectStorageName, Namespace: database.Namespace}, objectStorage)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to get object storage %s", schedule.ObjectStorageName)
		}

		storages[schedule.ObjectStorageName] = &pxcv1.BackupStorageSpec{
			Type: pxcv1.BackupStorageType(objectStorage.Spec.Type),
		}
		switch objectStorage.Spec.Type {
		case everestv1alpha1.ObjectStorageTypeS3:
			storages[schedule.ObjectStorageName].S3 = &pxcv1.BackupStorageS3Spec{
				Bucket:            objectStorage.Spec.Bucket,
				CredentialsSecret: objectStorage.Spec.CredentialsSecretName,
				Region:            objectStorage.Spec.Region,
				EndpointURL:       objectStorage.Spec.EndpointURL,
			}
		default:
			return nil, errors.Errorf("unsupported object storage type %s for %s", objectStorage.Spec.Type, objectStorage.Name)
		}

		pxcSchedules = append(pxcSchedules, pxcv1.PXCScheduledBackupSchedule{
			Name:        schedule.Name,
			Schedule:    schedule.Schedule,
			Keep:        int(schedule.RetentionCopies),
			StorageName: schedule.ObjectStorageName,
		})
	}

	pxcBackupSpec.Storages = storages
	pxcBackupSpec.Schedule = pxcSchedules

	return pxcBackupSpec, nil
}

func (r *DatabaseClusterReconciler) reconcilePXC(ctx context.Context, req ctrl.Request, database *everestv1alpha1.DatabaseCluster) error { //nolint:lll,gocognit,gocyclo,cyclop,maintidx
	version, err := r.getOperatorVersion(ctx, types.NamespacedName{
		Namespace: req.NamespacedName.Namespace,
		Name:      pxcDeploymentName,
	})
	if err != nil {
		return err
	}

	current := &pxcv1.PerconaXtraDBCluster{}
	err = r.Get(ctx, types.NamespacedName{Name: database.Name, Namespace: database.Namespace}, current)
	if err != nil {
		if err = client.IgnoreNotFound(err); err != nil {
			return err
		}
	}
	if current.Spec.Pause != database.Spec.Paused {
		// During the restoration of PXC clusters
		// They need to be shutted down
		//
		// It's not a good idea to shutdown them from DatabaseCluster object perspective
		// hence we have this piece of the migration of spec.pause field
		// from PerconaXtraDBCluster object to a DatabaseCluster object.

		restores := &everestv1alpha1.DatabaseClusterRestoreList{}

		if err := r.List(ctx, restores, client.MatchingFields{"spec.databaseCluster": database.Name}); err != nil {
			return err
		}
		jobRunning := false
		for _, restore := range restores.Items {
			switch restore.Status.State {
			case everestv1alpha1.RestoreState(pxcv1.RestoreNew):
				jobRunning = true
			case everestv1alpha1.RestoreState(pxcv1.RestoreStarting):
				jobRunning = true
			case everestv1alpha1.RestoreState(pxcv1.RestoreStopCluster):
				jobRunning = true
			case everestv1alpha1.RestoreState(pxcv1.RestoreRestore):
				jobRunning = true
			case everestv1alpha1.RestoreState(pxcv1.RestoreStartCluster):
				jobRunning = true
			case everestv1alpha1.RestoreState(pxcv1.RestorePITR):
				jobRunning = true
			default:
				jobRunning = false
			}
		}
		if jobRunning {
			database.Spec.Paused = current.Spec.Pause
		}
	}

	pxc := &pxcv1.PerconaXtraDBCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:        database.Name,
			Namespace:   database.Namespace,
			Annotations: database.Annotations,
		},
		Spec: defaultPXCSpec,
	}

	if len(database.Finalizers) != 0 {
		pxc.Finalizers = database.Finalizers
		database.Finalizers = []string{}
	}

	if database.Spec.Proxy.Type == "" {
		database.Spec.Proxy.Type = everestv1alpha1.ProxyTypeHAProxy
	}
	if database.Spec.Proxy.Type == everestv1alpha1.ProxyTypeHAProxy && database.Spec.Proxy.Config == "" {
		database.Spec.Proxy.Config = haProxyDefaultConfigurationTemplate
	}
	if err := r.Update(ctx, database); err != nil {
		return err
	}

	engine := &everestv1alpha1.DatabaseEngine{}
	err = r.Get(ctx, types.NamespacedName{Name: pxcDeploymentName, Namespace: database.Namespace}, engine)
	if err != nil {
		return errors.Wrapf(err, "failed to get database engine %s", pxcDeploymentName)
	}

	if err := controllerutil.SetControllerReference(database, pxc, r.Client.Scheme()); err != nil {
		return err
	}
	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, pxc, func() error {
		pxc.TypeMeta = metav1.TypeMeta{
			APIVersion: version.ToAPIVersion(pxcAPIGroup),
			Kind:       PerconaXtraDBClusterKind,
		}

		clusterType, err := r.getClusterType(ctx)
		if err != nil {
			return err
		}

		pxc.Spec = defaultPXCSpec
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
			return errors.Errorf("missing %s annotation", dbTemplateNameAnnotationKey)
		}
		if !hasTemplateKind && hasTemplateName {
			return errors.Errorf("missing %s annotation", dbTemplateKindAnnotationKey)
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

		pxc.Spec.CRVersion = version.ToCRVersion()
		pxc.Spec.AllowUnsafeConfig = database.Spec.Engine.Replicas == 1
		pxc.Spec.Pause = database.Spec.Paused
		pxc.Spec.SecretsName = database.Spec.Engine.UserSecretsName

		if database.Spec.Engine.Config != "" {
			pxc.Spec.PXC.PodSpec.Configuration = database.Spec.Engine.Config
		}
		if pxc.Spec.PXC.PodSpec.Configuration == "" {
			// Config missing from the DatabaseCluster CR and the template (if any), apply the default one
			gCacheSize := "600M"

			if database.Spec.Engine.Resources.Memory.CmpInt64(memorySmallSize) > 0 && database.Spec.Engine.Resources.Memory.CmpInt64(memoryMediumSize) <= 0 {
				gCacheSize = "2457M"
			}
			if database.Spec.Engine.Resources.Memory.CmpInt64(memoryMediumSize) > 0 && database.Spec.Engine.Resources.Memory.CmpInt64(memoryLargeSize) <= 0 {
				gCacheSize = "9830M"
			}
			if database.Spec.Engine.Resources.Memory.CmpInt64(memoryLargeSize) >= 0 {
				gCacheSize = "9830M"
			}
			ver, _ := goversion.NewVersion("v1.11.0")
			pxc.Spec.PXC.PodSpec.Configuration = fmt.Sprintf(pxcDefaultConfigurationTemplate, gCacheSize)
			if version.version.GreaterThan(ver) {
				pxc.Spec.PXC.PodSpec.Configuration = fmt.Sprintf(pxcMinimalConfigurationTemplate, gCacheSize)
			}
		}

		if database.Spec.Engine.Replicas == 0 {
			database.Spec.Engine.Replicas = 3
		}
		pxc.Spec.PXC.PodSpec.Size = database.Spec.Engine.Replicas

		if database.Spec.Engine.Version == "" {
			database.Spec.Engine.Version = engine.BestEngineVersion()
		}
		pxcEngineVersion, ok := engine.Status.AvailableVersions.Engine[database.Spec.Engine.Version]
		if !ok {
			return errors.Errorf("engine version %s not available", database.Spec.Engine.Version)
		}

		pxc.Spec.PXC.PodSpec.Image = pxcEngineVersion.ImagePath

		pxc.Spec.PXC.PodSpec.VolumeSpec = &pxcv1.VolumeSpec{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimSpec{
				StorageClassName: database.Spec.Engine.Storage.Class,
				Resources: corev1.ResourceRequirements{
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

		switch database.Spec.Proxy.Type {
		case everestv1alpha1.ProxyTypeHAProxy:
			pxc.Spec.ProxySQL.Enabled = false
			pxc.Spec.HAProxy, err = r.genPXCHAProxySpec(database, engine)
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
			return errors.Errorf("invalid proxy type %s", database.Spec.Proxy.Type)
		}

		monitoring := &everestv1alpha1.MonitoringConfig{}
		if database.Spec.Monitoring != nil && database.Spec.Monitoring.MonitoringConfigName != "" {
			err := r.Get(ctx, types.NamespacedName{
				Namespace: req.NamespacedName.Namespace,
				Name:      database.Spec.Monitoring.MonitoringConfigName,
			}, monitoring)
			if err != nil {
				return err
			}
		}

		if monitoring.Spec.Type == everestv1alpha1.PMM {
			pxc.Spec.PMM.Enabled = true
			pxc.Spec.PMM.ServerHost = monitoring.Spec.PMM.URL
			pxc.Spec.PMM.Image = monitoring.Spec.PMM.Image
			pxc.Spec.PMM.Resources = database.Spec.Monitoring.Resources

			apiKey, err := r.getSecretFromMonitoringConfig(ctx, database, monitoring, "apiKey")
			if err != nil {
				return err
			}

			err = r.createOrUpdateSecret(ctx, database, pxc.Spec.SecretsName, "", map[string][]byte{
				"pmmserverkey": []byte(apiKey),
			})
			if err != nil {
				return err
			}
		}

		if database.Spec.Backup.Enabled {
			pxc.Spec.Backup, err = r.genPXCBackupSpec(ctx, database, engine)
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	if database.Spec.DataSource != nil {
		err = r.reconcileDBRestoreFromDataSource(ctx, database)
		if err != nil {
			return err
		}
	}

	database.Status.Hostname = pxc.Status.Host
	database.Status.Status = everestv1alpha1.AppState(pxc.Status.Status)
	database.Status.Ready = pxc.Status.Ready
	database.Status.Size = pxc.Status.Size
	database.Status.Message = strings.Join(pxc.Status.Messages, ";")
	return r.Status().Update(ctx, database)
}

func (r *DatabaseClusterReconciler) createPGBackrestSecret(
	ctx context.Context,
	database *everestv1alpha1.DatabaseCluster,
	storageSecretName,
	repoName,
	pgBackrestSecretName string,
) (*corev1.Secret, error) {
	s3StorageSecret := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{Name: storageSecretName, Namespace: database.Namespace}, s3StorageSecret)
	if err != nil {
		return nil, err
	}

	pgbackrestS3Conf := fmt.Sprintf("[global]\n%s-s3-key=%s\n%s-s3-key-secret=%s\n",
		repoName, s3StorageSecret.Data["AWS_ACCESS_KEY_ID"],
		repoName, s3StorageSecret.Data["AWS_SECRET_ACCESS_KEY"],
	)
	pgBackrestSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pgBackrestSecretName,
			Namespace: database.Namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"s3.conf": []byte(pgbackrestS3Conf),
		},
	}
	err = controllerutil.SetControllerReference(database, pgBackrestSecret, r.Scheme)
	if err != nil {
		return nil, err
	}
	err = r.createOrUpdate(ctx, pgBackrestSecret)
	if err != nil {
		return nil, err
	}

	return pgBackrestSecret, nil
}

func (r *DatabaseClusterReconciler) genPGBackupsSpec(
	ctx context.Context,
	database *everestv1alpha1.DatabaseCluster,
	engine *everestv1alpha1.DatabaseEngine,
) (crunchyv1beta1.Backups, error) {
	pgbackrestVersion, ok := engine.Status.AvailableVersions.Backup[database.Spec.Engine.Version]
	if !ok {
		return crunchyv1beta1.Backups{}, errors.Errorf("pgbackrest version %s not available", database.Spec.Engine.Version)
	}

	backups := crunchyv1beta1.Backups{
		PGBackRest: crunchyv1beta1.PGBackRestArchive{
			Global: map[string]string{},
			Image:  pgbackrestVersion.ImagePath,
			Manual: &crunchyv1beta1.PGBackRestManualBackup{
				RepoName: "repo1",
			},
		},
	}
	if len(database.Spec.Backup.Schedules) > 4 {
		return crunchyv1beta1.Backups{}, errors.Errorf("number of backup schedules for postgresql cannot exceed 4")
	}

	repos := make([]crunchyv1beta1.PGBackRestRepo, len(database.Spec.Backup.Schedules))
	for idx, schedule := range database.Spec.Backup.Schedules {
		if !schedule.Enabled {
			continue
		}

		objectStorage := &everestv1alpha1.ObjectStorage{}
		err := r.Get(ctx, types.NamespacedName{Name: schedule.ObjectStorageName, Namespace: database.Namespace}, objectStorage)
		if err != nil {
			return crunchyv1beta1.Backups{}, errors.Wrapf(err, "failed to get object storage %s", schedule.ObjectStorageName)
		}

		repos[idx] = crunchyv1beta1.PGBackRestRepo{
			Name: fmt.Sprintf("repo%d", idx+1),
			BackupSchedules: &crunchyv1beta1.PGBackRestBackupSchedules{
				Full: &database.Spec.Backup.Schedules[idx].Schedule,
			},
		}
		backups.PGBackRest.Global[repos[idx].Name+"-retention-full"] = fmt.Sprintf("%d", database.Spec.Backup.Schedules[idx].RetentionCopies)

		switch objectStorage.Spec.Type {
		case everestv1alpha1.ObjectStorageTypeS3:
			repos[idx].S3 = &crunchyv1beta1.RepoS3{
				Bucket:   objectStorage.Spec.Bucket,
				Region:   objectStorage.Spec.Region,
				Endpoint: objectStorage.Spec.EndpointURL,
			}

			pgBackrestSecret, err := r.createPGBackrestSecret(
				ctx,
				database,
				objectStorage.Spec.CredentialsSecretName,
				repos[idx].Name,
				database.Name+"-pgbackrest-secrets",
			)
			if err != nil {
				return crunchyv1beta1.Backups{}, err
			}

			backups.PGBackRest.Configuration = []corev1.VolumeProjection{
				{
					Secret: &corev1.SecretProjection{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: pgBackrestSecret.Name,
						},
					},
				},
			}
		default:
			return crunchyv1beta1.Backups{}, errors.Errorf("unsupported object storage type %s for %s", objectStorage.Spec.Type, objectStorage.Name)
		}
	}
	backups.PGBackRest.Repos = repos
	return backups, nil
}

func (r *DatabaseClusterReconciler) genPGDataSourceSpec(ctx context.Context, database *everestv1alpha1.DatabaseCluster) (*crunchyv1beta1.DataSource, error) {
	destMatch := regexp.MustCompile(`/pgbackrest/(repo\d+)/backup/db/(.*)$`).FindStringSubmatch(database.Spec.DataSource.BackupName)
	if len(destMatch) < 3 {
		return nil, errors.Errorf("failed to extract the pgbackrest repo and backup names from %s", database.Spec.DataSource.BackupName)
	}
	repoName := destMatch[1]
	backupName := destMatch[2]

	pgDataSource := &crunchyv1beta1.DataSource{
		PGBackRest: &crunchyv1beta1.PGBackRestDataSource{
			Global: map[string]string{
				repoName + "-path": "/pgbackrest/" + repoName,
			},
			Stanza: "db",
			Options: []string{
				"--type=immediate",
				"--set=" + backupName,
			},
		},
	}

	objectStorage := &everestv1alpha1.ObjectStorage{}
	err := r.Get(ctx, types.NamespacedName{Name: database.Spec.DataSource.ObjectStorageName, Namespace: database.Namespace}, objectStorage)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to get object storage %s", database.Spec.DataSource.ObjectStorageName)
	}

	switch objectStorage.Spec.Type {
	case everestv1alpha1.ObjectStorageTypeS3:
		pgBackrestSecret, err := r.createPGBackrestSecret(
			ctx,
			database,
			objectStorage.Spec.CredentialsSecretName,
			repoName,
			database.Name+"-pgbackrest-datasource-secrets",
		)
		if err != nil {
			return nil, err
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
				Bucket:   objectStorage.Spec.Bucket,
				Endpoint: objectStorage.Spec.EndpointURL,
				Region:   objectStorage.Spec.Region,
			},
		}
	default:
		return nil, errors.Errorf("unsupported object storage type %s for %s", objectStorage.Spec.Type, objectStorage.Name)
	}
	return pgDataSource, nil
}

//nolint:gocognit,maintidx
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

	pgSpec := defaultPGSpec
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

	pg := &pgv2.PerconaPGCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:        database.Name,
			Namespace:   database.Namespace,
			Annotations: database.Annotations,
		},
		Spec: pgSpec,
	}

	if pg.Spec.PMM == nil {
		pg.Spec.PMM = &pgv2.PMMSpec{}
	}

	if pg.Spec.PMM.Secret == "" {
		pg.Spec.PMM.Secret = database.Spec.Engine.UserSecretsName
	}

	if len(database.Finalizers) != 0 {
		pg.Finalizers = database.Finalizers
		database.Finalizers = []string{}
	}
	if err := r.Update(ctx, database); err != nil {
		return err
	}

	engine := &everestv1alpha1.DatabaseEngine{}
	err = r.Get(ctx, types.NamespacedName{Name: pgDeploymentName, Namespace: database.Namespace}, engine)
	if err != nil {
		return errors.Wrapf(err, "failed to get database engine %s", pgDeploymentName)
	}
	if err := controllerutil.SetControllerReference(database, pg, r.Client.Scheme()); err != nil {
		return err
	}
	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, pg, func() error {
		pg.TypeMeta = metav1.TypeMeta{
			APIVersion: version.ToAPIVersion(pgAPIGroup),
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
			return errors.Errorf("engine version %s not available", database.Spec.Engine.Version)
		}

		pg.Spec.Image = pgEngineVersion.ImagePath

		pgMajorVersionMatch := regexp.MustCompile(`^(\d+)`).FindStringSubmatch(database.Spec.Engine.Version)
		if len(pgMajorVersionMatch) < 2 {
			return errors.Errorf("failed to extract the major version from %s", database.Spec.Engine.Version)
		}
		pgMajorVersion, err := strconv.Atoi(pgMajorVersionMatch[1])
		if err != nil {
			return err
		}
		pg.Spec.PostgresVersion = pgMajorVersion

		if database.Spec.Engine.Replicas == 0 {
			database.Spec.Engine.Replicas = 3
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
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: database.Spec.Engine.Storage.Size,
				},
			},
		}

		pgbouncerAvailVersions, ok := engine.Status.AvailableVersions.Proxy["pgbouncer"]
		if !ok {
			return errors.Errorf("pgbouncer version not available")
		}

		pgbouncerVersion, ok := pgbouncerAvailVersions[database.Spec.Engine.Version]
		if !ok {
			return errors.Errorf("pgbouncer version %s not available", database.Spec.Engine.Version)
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
			return errors.Errorf("invalid expose type %s", database.Spec.Proxy.Expose.Type)
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
				Namespace: req.NamespacedName.Namespace,
				Name:      database.Spec.Monitoring.MonitoringConfigName,
			}, monitoring)
			if err != nil {
				return err
			}
		}

		if monitoring.Spec.Type == everestv1alpha1.PMM {
			pg.Spec.PMM.Enabled = true
			pg.Spec.PMM.ServerHost = monitoring.Spec.PMM.URL
			pg.Spec.PMM.Image = monitoring.Spec.PMM.Image
			pg.Spec.PMM.Resources = database.Spec.Monitoring.Resources

			apiKey, err := r.getSecretFromMonitoringConfig(ctx, database, monitoring, "apiKey")
			if err != nil {
				return err
			}

			err = r.createOrUpdateSecret(ctx, database, pg.Spec.PMM.Secret, "", map[string][]byte{
				"PMM_SERVER_KEY": []byte(apiKey),
			})
			if err != nil {
				return err
			}
		}

		// If no backup is specified we need to define a repo regardless.
		// Without credentials need to define a PVC-backed repo because the
		// pg-operator requires a backup to be set up in order to create
		// replicas.
		pgbackrestVersion, ok := engine.Status.AvailableVersions.Backup[database.Spec.Engine.Version]
		if !ok {
			return errors.Errorf("pgbackrest version %s not available", database.Spec.Engine.Version)
		}
		pg.Spec.Backups = crunchyv1beta1.Backups{
			PGBackRest: crunchyv1beta1.PGBackRestArchive{
				Image: pgbackrestVersion.ImagePath,
				Repos: []crunchyv1beta1.PGBackRestRepo{
					{
						Name: "repo1",
						Volume: &crunchyv1beta1.RepoPVC{
							VolumeClaimSpec: corev1.PersistentVolumeClaimSpec{
								AccessModes: []corev1.PersistentVolumeAccessMode{
									corev1.ReadWriteOnce,
								},
								StorageClassName: database.Spec.Engine.Storage.Class,
								Resources: corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceStorage: database.Spec.Engine.Storage.Size,
									},
								},
							},
						},
					},
				},
				Manual: &crunchyv1beta1.PGBackRestManualBackup{
					RepoName: "repo1",
				},
			},
		}

		if database.Spec.Backup.Enabled {
			pg.Spec.Backups, err = r.genPGBackupsSpec(ctx, database, engine)
			if err != nil {
				return err
			}
		}

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

	database.Status.Hostname = pg.Status.Host
	database.Status.Status = everestv1alpha1.AppState(pg.Status.State)
	database.Status.Ready = pg.Status.Postgres.Ready + pg.Status.PGBouncer.Ready
	database.Status.Size = pg.Status.Postgres.Size + pg.Status.PGBouncer.Size
	return r.Status().Update(ctx, database)
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
	version, err := r.getOperatorVersion(context.Background(), types.NamespacedName{
		Name:      pxcDeploymentName,
		Namespace: os.Getenv("WATCH_NAMESPACE"),
	})
	if err != nil {
		return err
	}
	pxcSchemeGroupVersion := schema.GroupVersion{Group: pxcAPIGroup, Version: strings.ReplaceAll("v"+version.String(), ".", "-")}
	ver, _ := goversion.NewVersion("v1.11.0")
	if version.version.GreaterThan(ver) {
		pxcSchemeGroupVersion = schema.GroupVersion{Group: pxcAPIGroup, Version: "v1"}
	}

	scheme.AddKnownTypes(pxcSchemeGroupVersion,
		&pxcv1.PerconaXtraDBCluster{}, &pxcv1.PerconaXtraDBClusterList{})

	metav1.AddToGroupVersion(scheme, pxcSchemeGroupVersion)
	return nil
}

func (r *DatabaseClusterReconciler) addPSMDBKnownTypes(scheme *runtime.Scheme) error {
	version, err := r.getOperatorVersion(context.Background(), types.NamespacedName{
		Name:      psmdbDeploymentName,
		Namespace: os.Getenv("WATCH_NAMESPACE"),
	})
	if err != nil {
		return err
	}
	psmdbSchemeGroupVersion := schema.GroupVersion{Group: psmdbAPIGroup, Version: strings.ReplaceAll("v"+version.String(), ".", "-")}
	ver, _ := goversion.NewVersion("v1.12.0")
	if version.version.GreaterThan(ver) {
		psmdbSchemeGroupVersion = schema.GroupVersion{Group: psmdbAPIGroup, Version: "v1"}
	}
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
func (r *DatabaseClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
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

	return controller.Complete(r)
}

func (r *DatabaseClusterReconciler) initIndexers(ctx context.Context, mgr ctrl.Manager) error {
	// Index the ObjectStorage's CredentialsSecretName field so that it can be
	// used by the databaseClustersThatReferenceCredentialsSecret function to
	// find all DatabaseClusters that reference a specific secret through the
	// ObjectStorage's CredentialsSecretName field
	err := mgr.GetFieldIndexer().IndexField(ctx, &everestv1alpha1.ObjectStorage{}, credentialsSecretNameField, func(o client.Object) []string {
		var res []string
		objectStorage, ok := o.(*everestv1alpha1.ObjectStorage)
		if !ok {
			return res
		}
		res = append(res, objectStorage.Spec.CredentialsSecretName)
		return res
	})
	if err != nil {
		return err
	}

	// Index the ObjectStorageName so that it can be used by the
	// databaseClustersThatReferenceObjectStorage function to find all
	// DatabaseClusters that reference a specific ObjectStorage through the
	// ObjectStorageName field
	err = mgr.GetFieldIndexer().IndexField(
		ctx, &everestv1alpha1.DatabaseCluster{}, objectStorageNameField,
		func(o client.Object) []string {
			var res []string
			database, ok := o.(*everestv1alpha1.DatabaseCluster)
			if !ok || !database.Spec.Backup.Enabled {
				return res
			}
			for _, storage := range database.Spec.Backup.Schedules {
				res = append(res, storage.ObjectStorageName)
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
			res = append(res, dc.Spec.Monitoring.MonitoringConfigName)
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
	controller.Owns(&everestv1alpha1.ObjectStorage{})
	controller.Watches(
		&everestv1alpha1.ObjectStorage{},
		handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
			return r.databaseClustersThatReferenceObject(ctx, objectStorageNameField, obj)
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

	// ObjectStorage
	var res []reconcile.Request
	osList := &everestv1alpha1.ObjectStorageList{}
	err := r.findObjectsBySecretName(ctx, secret, credentialsSecretNameField, osList)
	if err != nil {
		logger.Error(err, "could not find ObjectStorage by secret name")
	}
	if err == nil {
		var items []client.Object
		for _, i := range osList.Items {
			i := i
			items = append(items, &i)
		}
		res = append(res, r.getDBClustersReconcileRequestsByRelatedObjectName(ctx, items, objectStorageNameField)...)
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
			return errors.Errorf("type mismatch for %s.%s, %T != %T", parent, k, dst[k], v)
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
				return errors.Errorf("type mismatch for %s.%s, %T != %T", parent, k, dst[k], v)
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

func (r *DatabaseClusterReconciler) createOrUpdate(ctx context.Context, obj client.Object) error {
	hash, err := getObjectHash(obj)
	if err != nil {
		return errors.Wrap(err, "calculate object hash")
	}

	val := reflect.ValueOf(obj)
	if val.Kind() == reflect.Ptr {
		val = reflect.Indirect(val)
	}
	oldObject, ok := reflect.New(val.Type()).Interface().(client.Object)
	if !ok {
		return errors.Errorf("failed type conversion")
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
		return errors.Wrap(err, "get object")
	}

	oldHash, err := getObjectHash(oldObject)
	if err != nil {
		return errors.Wrap(err, "calculate old object hash")
	}

	if oldHash != hash ||
		!isObjectMetaEqual(obj, oldObject) {
		obj.SetResourceVersion(oldObject.GetResourceVersion())
		switch object := obj.(type) {
		case *corev1.Service:
			oldObjectService, ok := oldObject.(*corev1.Service)
			if !ok {
				return errors.Errorf("failed type conversion")
			}
			object.Spec.ClusterIP = oldObjectService.Spec.ClusterIP
			if object.Spec.Type == corev1.ServiceTypeLoadBalancer {
				object.Spec.HealthCheckNodePort = oldObjectService.Spec.HealthCheckNodePort
			}
		default:
		}

		return r.Update(ctx, obj)
	}

	return nil
}
