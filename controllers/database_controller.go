// dbaas-operator
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

// Package controllers contains a set of controllers for DBaaS
package controllers

import (
	"context"
	"fmt"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/AlekSi/pointer"
	goversion "github.com/hashicorp/go-version"
	"github.com/percona/percona-backup-mongodb/pbm"
	pgv2beta1 "github.com/percona/percona-postgresql-operator/pkg/apis/pg.percona.com/v2beta1"
	crunchyv1beta1 "github.com/percona/percona-postgresql-operator/pkg/apis/postgres-operator.crunchydata.com/v1beta1"
	psmdbv1 "github.com/percona/percona-server-mongodb-operator/pkg/apis/psmdb/v1"
	pxcv1 "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/pkg/errors"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	dbaasv1 "github.com/percona/dbaas-operator/api/v1"
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
	pxcBackupImageTmpl  = "percona/percona-xtradb-cluster-operator:%s-pxc8.0-backup"

	psmdbCRDName                = "perconaservermongodbs.psmdb.percona.com"
	pxcCRDName                  = "perconaxtradbclusters.pxc.percona.com"
	pgCRDName                   = "perconapgclusters.pg.percona.com"
	pxcAPIGroup                 = "pxc.percona.com"
	psmdbAPIGroup               = "psmdb.percona.com"
	pgAPIGroup                  = "pg.percona.com"
	haProxyTemplate             = "percona/percona-xtradb-cluster-operator:%s-haproxy"
	restartAnnotationKey        = "dbaas.percona.com/restart"
	dbTemplateKindAnnotationKey = "dbaas.percona.com/dbtemplate-kind"
	dbTemplateNameAnnotationKey = "dbaas.percona.com/dbtemplate-name"
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
)

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
		},
	},
	ProxySQL: &pxcv1.PodSpec{
		Enabled: false,
		Affinity: &pxcv1.PodAffinity{
			TopologyKey: pointer.ToString(pxcv1.AffinityTopologyKeyOff),
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
				},
			},
		},
	}
)

var defaultPGSpec = pgv2beta1.PerconaPGClusterSpec{
	InstanceSets: pgv2beta1.PGInstanceSets{
		{
			Name: "instance1",
		},
	},
	PMM: &pgv2beta1.PMMSpec{
		Enabled: false,
		Resources: corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("300M"),
				corev1.ResourceCPU:    resource.MustParse("500m"),
			},
		},
	},
	Proxy: &pgv2beta1.PGProxySpec{
		PGBouncer: &pgv2beta1.PGBouncerSpec{},
	},
}

// DatabaseReconciler reconciles a Database object.
type DatabaseReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=dbaas.percona.com,resources=databaseclusters,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=dbaas.percona.com,resources=databaseclusters/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=dbaas.percona.com,resources=databaseclusters/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch
//+kubebuilder:rbac:groups=storage.k8s.io,resources=storageclasses,verbs=get;list;watch
//+kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch
//+kubebuilder:rbac:groups=pxc.percona.com,resources=perconaxtradbclusters,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=psmdb.percona.com,resources=perconaservermongodbs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=pg.percona.com,resources=perconapgclusters,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.12.2/pkg/reconcile
func (r *DatabaseReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling", "request", req)

	database := &dbaasv1.DatabaseCluster{}

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

	if ok && !database.Spec.Pause {
		database.Spec.Pause = true
	}
	if ok && database.Status.State == dbaasv1.AppStatePaused {
		database.Spec.Pause = false
		delete(database.ObjectMeta.Annotations, restartAnnotationKey)
		if err := r.Update(ctx, database); err != nil {
			return reconcile.Result{}, err
		}
	}
	if database.Spec.Database == "pxc" {
		err := r.reconcilePXC(ctx, req, database)
		return reconcile.Result{}, err
	}
	if database.Spec.Database == "psmdb" {
		err := r.reconcilePSMDB(ctx, req, database)
		if err != nil {
			logger.Error(err, "unable to reconcile psmdb")
		}
		return reconcile.Result{}, err
	}
	if database.Spec.Database == "postgresql" {
		err := r.reconcilePG(ctx, req, database)
		return reconcile.Result{}, err
	}
	return reconcile.Result{RequeueAfter: 5 * time.Second}, nil
}

func (r *DatabaseReconciler) getClusterType(ctx context.Context) (ClusterType, error) {
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

func (r *DatabaseReconciler) reconcilePSMDB(ctx context.Context, req ctrl.Request, database *dbaasv1.DatabaseCluster) error { //nolint:gocognit,maintidx
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
			Finalizers:  database.Finalizers,
			Annotations: database.Annotations,
		},
		Spec: defaultPSMDBSpec,
	}
	if err := r.Update(ctx, database); err != nil {
		return err
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
		psmdb.Spec.UnsafeConf = database.Spec.ClusterSize == 1
		psmdb.Spec.Pause = database.Spec.Pause
		psmdb.Spec.Image = database.Spec.DatabaseImage
		psmdb.Spec.Secrets = &psmdbv1.SecretsSpec{
			Users: database.Spec.SecretsName,
		}
		psmdb.Spec.Mongod.Security.EncryptionKeySecret = fmt.Sprintf("%s-mongodb-encryption-key", database.Name)

		if database.Spec.DatabaseConfig != "" {
			psmdb.Spec.Replsets[0].Configuration = psmdbv1.MongoConfiguration(database.Spec.DatabaseConfig)
		}
		if psmdb.Spec.Replsets[0].Configuration == "" {
			// Config missing from the DatabaseCluster CR and the template (if any), apply the default one
			psmdb.Spec.Replsets[0].Configuration = psmdbv1.MongoConfiguration(psmdbDefaultConfigurationTemplate)
		}

		psmdb.Spec.Replsets[0].Size = database.Spec.ClusterSize
		psmdb.Spec.Replsets[0].VolumeSpec = &psmdbv1.VolumeSpec{
			PersistentVolumeClaim: psmdbv1.PVCSpec{
				PersistentVolumeClaimSpec: &corev1.PersistentVolumeClaimSpec{
					StorageClassName: database.Spec.DBInstance.StorageClassName,
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: database.Spec.DBInstance.DiskSize,
						},
					},
				},
			},
		}
		psmdb.Spec.Replsets[0].MultiAZ.Resources = corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    database.Spec.DBInstance.CPU,
				corev1.ResourceMemory: database.Spec.DBInstance.Memory,
			},
		}
		psmdb.Spec.Sharding.ConfigsvrReplSet.Size = database.Spec.ClusterSize
		psmdb.Spec.Sharding.ConfigsvrReplSet.VolumeSpec = &psmdbv1.VolumeSpec{
			PersistentVolumeClaim: psmdbv1.PVCSpec{
				PersistentVolumeClaimSpec: &corev1.PersistentVolumeClaimSpec{
					StorageClassName: database.Spec.DBInstance.StorageClassName,
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: database.Spec.DBInstance.DiskSize,
						},
					},
				},
			},
		}
		psmdb.Spec.Sharding.Mongos.Size = database.Spec.LoadBalancer.Size
		psmdb.Spec.Sharding.Mongos.Expose = psmdbv1.MongosExpose{
			Expose: psmdbv1.Expose{
				ExposeType:               database.Spec.LoadBalancer.ExposeType,
				LoadBalancerSourceRanges: database.Spec.LoadBalancer.LoadBalancerSourceRanges,
				ServiceAnnotations:       database.Spec.LoadBalancer.Annotations,
			},
		}
		psmdb.Spec.Sharding.Mongos.Configuration = psmdbv1.MongoConfiguration(database.Spec.LoadBalancer.Configuration)
		psmdb.Spec.Sharding.Mongos.MultiAZ.Resources = database.Spec.LoadBalancer.Resources
		if database.Spec.ClusterSize == 1 {
			psmdb.Spec.Sharding.Enabled = false
			psmdb.Spec.Replsets[0].Expose.Enabled = true
			psmdb.Spec.Replsets[0].Expose.ExposeType = database.Spec.LoadBalancer.ExposeType
			psmdb.Spec.Sharding.Mongos.Expose.ExposeType = corev1.ServiceTypeClusterIP
		}
		if database.Spec.Monitoring.PMM != nil {
			psmdb.Spec.PMM.Enabled = true
			psmdb.Spec.PMM.ServerHost = database.Spec.Monitoring.PMM.PublicAddress
			psmdb.Spec.PMM.Image = database.Spec.Monitoring.PMM.Image
		}
		if database.Spec.Backup != nil {
			backupType := pbm.LogicalBackup
			if database.Spec.Backup.Type == string(pbm.PhysicalBackup) {
				backupType = pbm.PhysicalBackup
			}

			if database.Spec.Backup.Image == "" {
				image, err := version.PSMDBBackupImage()
				if err != nil {
					return err
				}
				database.Spec.Backup.Image = image
			}
			psmdb.Spec.Backup = psmdbv1.BackupSpec{
				Enabled:                  true,
				Image:                    database.Spec.Backup.Image,
				ServiceAccountName:       database.Spec.Backup.ServiceAccountName,
				ContainerSecurityContext: database.Spec.Backup.ContainerSecurityContext,
				Resources:                database.Spec.Backup.Resources,
				Annotations:              database.Spec.Backup.Annotations,
				Labels:                   database.Spec.Backup.Labels,
			}
			storages := make(map[string]psmdbv1.BackupStorageSpec)
			var tasks []psmdbv1.BackupTaskSpec
			for k, v := range database.Spec.Backup.Storages {
				switch v.Type {
				case dbaasv1.BackupStorageS3:
					storages[k] = psmdbv1.BackupStorageSpec{
						Type: psmdbv1.BackupStorageType(v.Type),
						S3: psmdbv1.BackupStorageS3Spec{
							Bucket:            v.StorageProvider.Bucket,
							CredentialsSecret: v.StorageProvider.CredentialsSecret,
							Region:            v.StorageProvider.Region,
							EndpointURL:       v.StorageProvider.EndpointURL,
							StorageClass:      v.StorageProvider.StorageClass,
						},
					}
				case dbaasv1.BackupStorageAzure:
					storages[k] = psmdbv1.BackupStorageSpec{
						Type: psmdbv1.BackupStorageType(v.Type),
						Azure: psmdbv1.BackupStorageAzureSpec{
							Container:         v.StorageProvider.ContainerName,
							CredentialsSecret: v.StorageProvider.CredentialsSecret,
							Prefix:            v.StorageProvider.Prefix,
						},
					}
				}
			}
			for _, v := range database.Spec.Backup.Schedule {
				tasks = append(tasks, psmdbv1.BackupTaskSpec{
					Name:             v.Name,
					Enabled:          v.Enabled,
					Keep:             v.Keep,
					Schedule:         v.Schedule,
					StorageName:      v.StorageName,
					CompressionType:  v.CompressionType,
					CompressionLevel: v.CompressionLevel,
					Type:             backupType,
				})
			}
			psmdb.Spec.Backup.Storages = storages
			psmdb.Spec.Backup.Tasks = tasks
		}
		return nil
	})
	if err != nil {
		return err
	}
	database.Status.Host = psmdb.Status.Host
	database.Status.Ready = psmdb.Status.Ready
	database.Status.Size = psmdb.Status.Size
	database.Status.State = dbaasv1.AppState(psmdb.Status.State)
	message := psmdb.Status.Message
	conditions := psmdb.Status.Conditions
	if message == "" && len(conditions) != 0 {
		message = conditions[len(conditions)-1].Message
	}
	database.Status.Message = message

	return r.Status().Update(ctx, database)
}

func (r *DatabaseReconciler) reconcilePXC(ctx context.Context, req ctrl.Request, database *dbaasv1.DatabaseCluster) error { //nolint:gocognit,gocyclo,cyclop,maintidx
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
	if current.Spec.Pause != database.Spec.Pause {
		// During the restoration of PXC clusters
		// They need to be shutted down
		//
		// It's not a good idea to shutdown them from DatabaseCluster object perspective
		// hence we have this piece of the migration of spec.pause field
		// from PerconaXtraDBCluster object to a DatabaseCluster object.

		restores := &dbaasv1.DatabaseClusterRestoreList{}

		if err := r.List(ctx, restores, client.MatchingFields{"spec.databaseCluster": database.Name}); err != nil {
			return err
		}
		jobRunning := false
		for _, restore := range restores.Items {
			switch restore.Status.State {
			case dbaasv1.RestoreState(pxcv1.RestoreNew):
				jobRunning = true
			case dbaasv1.RestoreState(pxcv1.RestoreStarting):
				jobRunning = true
			case dbaasv1.RestoreState(pxcv1.RestoreStopCluster):
				jobRunning = true
			case dbaasv1.RestoreState(pxcv1.RestoreRestore):
				jobRunning = true
			case dbaasv1.RestoreState(pxcv1.RestoreStartCluster):
				jobRunning = true
			case dbaasv1.RestoreState(pxcv1.RestorePITR):
				jobRunning = true
			default:
				jobRunning = false
			}
		}
		if jobRunning {
			database.Spec.Pause = current.Spec.Pause
		}
	}

	pxc := &pxcv1.PerconaXtraDBCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:        database.Name,
			Namespace:   database.Namespace,
			Finalizers:  database.Finalizers,
			Annotations: database.Annotations,
		},
		Spec: defaultPXCSpec,
	}
	if database.Spec.LoadBalancer.Type == "haproxy" && database.Spec.LoadBalancer.Configuration == "" {
		database.Spec.LoadBalancer.Configuration = haProxyDefaultConfigurationTemplate
	}
	if err := r.Update(ctx, database); err != nil {
		return err
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
		pxc.Spec.AllowUnsafeConfig = database.Spec.ClusterSize == 1
		pxc.Spec.Pause = database.Spec.Pause
		pxc.Spec.SecretsName = database.Spec.SecretsName

		if database.Spec.DatabaseConfig != "" {
			pxc.Spec.PXC.PodSpec.Configuration = database.Spec.DatabaseConfig
		}
		if pxc.Spec.PXC.PodSpec.Configuration == "" {
			// Config missing from the DatabaseCluster CR and the template (if any), apply the default one
			gCacheSize := "600M"

			if database.Spec.DBInstance.Memory.CmpInt64(memorySmallSize) > 0 && database.Spec.DBInstance.Memory.CmpInt64(memoryMediumSize) <= 0 {
				gCacheSize = "2457M"
			}
			if database.Spec.DBInstance.Memory.CmpInt64(memoryMediumSize) > 0 && database.Spec.DBInstance.Memory.CmpInt64(memoryLargeSize) <= 0 {
				gCacheSize = "9830M"
			}
			if database.Spec.DBInstance.Memory.CmpInt64(memoryLargeSize) >= 0 {
				gCacheSize = "9830M"
			}
			ver, _ := goversion.NewVersion("v1.11.0")
			pxc.Spec.PXC.PodSpec.Configuration = fmt.Sprintf(pxcDefaultConfigurationTemplate, gCacheSize)
			if version.version.GreaterThan(ver) {
				pxc.Spec.PXC.PodSpec.Configuration = fmt.Sprintf(pxcMinimalConfigurationTemplate, gCacheSize)
			}
		}

		pxc.Spec.PXC.PodSpec.Size = database.Spec.ClusterSize
		pxc.Spec.PXC.PodSpec.Image = database.Spec.DatabaseImage
		pxc.Spec.PXC.PodSpec.VolumeSpec = &pxcv1.VolumeSpec{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimSpec{
				StorageClassName: database.Spec.DBInstance.StorageClassName,
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: database.Spec.DBInstance.DiskSize,
					},
				},
			},
		}
		pxc.Spec.PXC.PodSpec.Resources = corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    database.Spec.DBInstance.CPU,
				corev1.ResourceMemory: database.Spec.DBInstance.Memory,
			},
		}
		if database.Spec.LoadBalancer.Type == "haproxy" {
			pxc.Spec.ProxySQL.Enabled = false

			if database.Spec.LoadBalancer.Image == "" {
				database.Spec.LoadBalancer.Image = fmt.Sprintf(haProxyTemplate, version.String())
			}
			pxc.Spec.HAProxy.PodSpec.Size = database.Spec.LoadBalancer.Size
			pxc.Spec.HAProxy.PodSpec.ServiceType = database.Spec.LoadBalancer.ExposeType
			pxc.Spec.HAProxy.PodSpec.ReplicasServiceType = database.Spec.LoadBalancer.ExposeType
			pxc.Spec.HAProxy.PodSpec.Configuration = database.Spec.LoadBalancer.Configuration
			pxc.Spec.HAProxy.PodSpec.LoadBalancerSourceRanges = database.Spec.LoadBalancer.LoadBalancerSourceRanges
			pxc.Spec.HAProxy.PodSpec.Annotations = database.Spec.LoadBalancer.Annotations
			pxc.Spec.HAProxy.PodSpec.ExternalTrafficPolicy = database.Spec.LoadBalancer.TrafficPolicy
			pxc.Spec.HAProxy.PodSpec.ReplicasExternalTrafficPolicy = database.Spec.LoadBalancer.TrafficPolicy
			pxc.Spec.HAProxy.PodSpec.Resources = database.Spec.LoadBalancer.Resources
			pxc.Spec.HAProxy.PodSpec.Enabled = true
			pxc.Spec.HAProxy.PodSpec.Image = database.Spec.LoadBalancer.Image
		}
		if database.Spec.LoadBalancer.Type == "proxysql" {
			pxc.Spec.HAProxy.PodSpec.Enabled = false

			pxc.Spec.ProxySQL.Size = database.Spec.LoadBalancer.Size
			pxc.Spec.ProxySQL.ServiceType = database.Spec.LoadBalancer.ExposeType
			pxc.Spec.ProxySQL.Configuration = database.Spec.LoadBalancer.Configuration
			pxc.Spec.ProxySQL.LoadBalancerSourceRanges = database.Spec.LoadBalancer.LoadBalancerSourceRanges
			pxc.Spec.ProxySQL.Annotations = database.Spec.LoadBalancer.Annotations
			pxc.Spec.ProxySQL.ExternalTrafficPolicy = database.Spec.LoadBalancer.TrafficPolicy
			pxc.Spec.ProxySQL.Resources = database.Spec.LoadBalancer.Resources
			pxc.Spec.ProxySQL.Enabled = true
			pxc.Spec.ProxySQL.Image = database.Spec.LoadBalancer.Image
		}
		if database.Spec.Monitoring.PMM != nil {
			pxc.Spec.PMM.Enabled = true
			pxc.Spec.PMM.ServerHost = database.Spec.Monitoring.PMM.PublicAddress
			pxc.Spec.PMM.ServerUser = database.Spec.Monitoring.PMM.Login
			pxc.Spec.PMM.Image = database.Spec.Monitoring.PMM.Image
		}
		if database.Spec.Backup != nil {
			if database.Spec.Backup.Image == "" {
				database.Spec.Backup.Image = fmt.Sprintf(pxcBackupImageTmpl, pxc.Spec.CRVersion)
			}
			pxc.Spec.Backup = &pxcv1.PXCScheduledBackup{
				Image:              database.Spec.Backup.Image,
				ImagePullSecrets:   database.Spec.Backup.ImagePullSecrets,
				ImagePullPolicy:    database.Spec.Backup.ImagePullPolicy,
				ServiceAccountName: database.Spec.Backup.ServiceAccountName,
			}
			storages := make(map[string]*pxcv1.BackupStorageSpec)
			var schedules []pxcv1.PXCScheduledBackupSchedule
			for k, v := range database.Spec.Backup.Storages {
				storages[k] = &pxcv1.BackupStorageSpec{
					Type:                     pxcv1.BackupStorageType(v.Type),
					NodeSelector:             v.NodeSelector,
					Resources:                v.Resources,
					Affinity:                 v.Affinity,
					Tolerations:              v.Tolerations,
					Annotations:              v.Annotations,
					Labels:                   v.Labels,
					SchedulerName:            v.SchedulerName,
					PriorityClassName:        v.PriorityClassName,
					PodSecurityContext:       v.PodSecurityContext,
					ContainerSecurityContext: v.ContainerSecurityContext,
					RuntimeClassName:         v.RuntimeClassName,
					VerifyTLS:                v.VerifyTLS,
				}
				switch v.Type {
				case dbaasv1.BackupStorageS3:
					storages[k].S3 = &pxcv1.BackupStorageS3Spec{
						Bucket:            v.StorageProvider.Bucket,
						CredentialsSecret: v.StorageProvider.CredentialsSecret,
						Region:            v.StorageProvider.Region,
						EndpointURL:       v.StorageProvider.EndpointURL,
					}
				case dbaasv1.BackupStorageAzure:
					storages[k].Azure = &pxcv1.BackupStorageAzureSpec{
						ContainerPath:     v.StorageProvider.ContainerName,
						CredentialsSecret: v.StorageProvider.CredentialsSecret,
						StorageClass:      v.StorageProvider.StorageClass,
						Endpoint:          v.StorageProvider.EndpointURL,
					}
				}
			}
			for _, v := range database.Spec.Backup.Schedule {
				schedules = append(schedules, pxcv1.PXCScheduledBackupSchedule{
					Name:        v.Name,
					Schedule:    v.Schedule,
					Keep:        v.Keep,
					StorageName: v.StorageName,
				})
			}
			pxc.Spec.Backup.Storages = storages
			pxc.Spec.Backup.Schedule = schedules
		}
		return nil
	})
	if err != nil {
		return err
	}
	database.Status.Host = pxc.Status.Host
	database.Status.State = dbaasv1.AppState(pxc.Status.Status)
	database.Status.Ready = pxc.Status.Ready
	database.Status.Size = pxc.Status.Size
	database.Status.Message = strings.Join(pxc.Status.Messages, ";")

	return r.Status().Update(ctx, database)
}

func (r *DatabaseReconciler) reconcilePG(ctx context.Context, _ ctrl.Request, database *dbaasv1.DatabaseCluster) error {
	version, err := NewVersion("v2beta1")
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

	pg := &pgv2beta1.PerconaPGCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:        database.Name,
			Namespace:   database.Namespace,
			Finalizers:  database.Finalizers,
			Annotations: database.Annotations,
		},
		Spec: pgSpec,
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
		// pg.Spec.SecretsName = database.Spec.SecretsName
		pg.Spec.Shutdown = &database.Spec.Pause
		pg.Spec.Image = database.Spec.DatabaseImage
		pgVersionMatch := regexp.MustCompile(`-ppg(\d+)-`).FindStringSubmatch(database.Spec.DatabaseImage)
		if len(pgVersionMatch) < 2 {
			return errors.Errorf("failed to extract the PostgresVersion from %s", database.Spec.DatabaseImage)
		}
		pgVersion, err := strconv.Atoi(pgVersionMatch[1])
		if err != nil {
			return err
		}
		pg.Spec.PostgresVersion = pgVersion

		//nolint:godox
		// FIXME This backup section has a hardcoded image field and a
		// PVC-backed repo because the pg-operator requires a backup to be set
		// up in order to create replicas. This needs to be fixed when
		// implementing the entire backup feature.
		// https://jira.percona.com/browse/K8SPG-297
		pg.Spec.Backups = crunchyv1beta1.Backups{
			PGBackRest: crunchyv1beta1.PGBackRestArchive{
				Image: fmt.Sprintf("percona/percona-postgresql-operator:2.0.0-ppg%d-pgbackrest", pgVersion),
				Repos: []crunchyv1beta1.PGBackRestRepo{
					{
						Name: "repo1",
						Volume: &crunchyv1beta1.RepoPVC{
							VolumeClaimSpec: corev1.PersistentVolumeClaimSpec{
								AccessModes: []corev1.PersistentVolumeAccessMode{
									corev1.ReadWriteOnce,
								},
								StorageClassName: database.Spec.DBInstance.StorageClassName,
								Resources: corev1.ResourceRequirements{
									Requests: corev1.ResourceList{
										corev1.ResourceStorage: database.Spec.DBInstance.DiskSize,
									},
								},
							},
						},
					},
				},
			},
		}

		pg.Spec.InstanceSets[0].Replicas = &database.Spec.ClusterSize
		pg.Spec.InstanceSets[0].Resources = corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    database.Spec.DBInstance.CPU,
				corev1.ResourceMemory: database.Spec.DBInstance.Memory,
			},
		}
		pg.Spec.InstanceSets[0].DataVolumeClaimSpec = corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			},
			StorageClassName: database.Spec.DBInstance.StorageClassName,
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: database.Spec.DBInstance.DiskSize,
				},
			},
		}

		pg.Spec.Proxy.PGBouncer.Image = database.Spec.LoadBalancer.Image
		pg.Spec.Proxy.PGBouncer.Replicas = &database.Spec.LoadBalancer.Size
		//nolint:godox
		// TODO add support for database.Spec.LoadBalancer.LoadBalancerSourceRanges
		// https://jira.percona.com/browse/K8SPG-311
		pg.Spec.Proxy.PGBouncer.ServiceExpose = &pgv2beta1.ServiceExpose{
			Metadata: crunchyv1beta1.Metadata{
				Annotations: database.Spec.LoadBalancer.Annotations,
			},
			Type: string(database.Spec.LoadBalancer.ExposeType),
		}
		pg.Spec.Proxy.PGBouncer.Resources = database.Spec.LoadBalancer.Resources

		if database.Spec.Monitoring.PMM != nil {
			pg.Spec.PMM.Enabled = true
			pg.Spec.PMM.ServerHost = database.Spec.Monitoring.PMM.PublicAddress
			pg.Spec.PMM.Image = database.Spec.Monitoring.PMM.Image
		}
		return nil
	})
	if err != nil {
		return err
	}

	//nolint:godox
	// FIXME the status updates below are made as a hacky workaround until
	// https://jira.percona.com/browse/K8SPG-295 is addressed.
	database.Status.Ready = 0
	database.Status.Size = 0
	for _, instanceStatus := range pg.Status.InstanceSets {
		database.Status.Ready += instanceStatus.ReadyReplicas
		database.Status.Size += instanceStatus.Replicas
	}
	database.Status.Ready += pg.Status.Proxy.PGBouncer.ReadyReplicas
	database.Status.Size += pg.Status.Proxy.PGBouncer.Replicas
	database.Status.Host = fmt.Sprintf("%s-pgbouncer.%s.svc", database.Name, database.Namespace)

	database.Status.State = dbaasv1.AppStateInit
	if database.Status.Ready != 0 && database.Status.Ready == database.Status.Size {
		database.Status.State = dbaasv1.AppStateReady
	}
	if database.Spec.Pause {
		database.Status.State = dbaasv1.AppStateStopping
	}

	var message string
	conditions := pg.Status.Conditions
	if len(conditions) != 0 {
		message = conditions[len(conditions)-1].Message
	}
	database.Status.Message = message

	if err := r.Status().Update(ctx, database); err != nil {
		return err
	}

	return nil
}

func (r *DatabaseReconciler) getOperatorVersion(ctx context.Context, name types.NamespacedName) (*Version, error) {
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

func (r *DatabaseReconciler) addPXCKnownTypes(scheme *runtime.Scheme) error {
	version, err := r.getOperatorVersion(context.Background(), types.NamespacedName{
		Name:      pxcDeploymentName,
		Namespace: os.Getenv("WATCH_NAMESPACE"),
	})
	if err != nil {
		return err
	}
	pxcSchemeGroupVersion := schema.GroupVersion{Group: "pxc.percona.com", Version: strings.ReplaceAll("v"+version.String(), ".", "-")}
	ver, _ := goversion.NewVersion("v1.11.0")
	if version.version.GreaterThan(ver) {
		pxcSchemeGroupVersion = schema.GroupVersion{Group: "pxc.percona.com", Version: "v1"}
	}

	scheme.AddKnownTypes(pxcSchemeGroupVersion,
		&pxcv1.PerconaXtraDBCluster{}, &pxcv1.PerconaXtraDBClusterList{})

	metav1.AddToGroupVersion(scheme, pxcSchemeGroupVersion)
	return nil
}

func (r *DatabaseReconciler) addPSMDBKnownTypes(scheme *runtime.Scheme) error {
	version, err := r.getOperatorVersion(context.Background(), types.NamespacedName{
		Name:      psmdbDeploymentName,
		Namespace: os.Getenv("WATCH_NAMESPACE"),
	})
	if err != nil {
		return err
	}
	psmdbSchemeGroupVersion := schema.GroupVersion{Group: "psmdb.percona.com", Version: strings.ReplaceAll("v"+version.String(), ".", "-")}
	ver, _ := goversion.NewVersion("v1.12.0")
	if version.version.GreaterThan(ver) {
		psmdbSchemeGroupVersion = schema.GroupVersion{Group: "psmdb.percona.com", Version: "v1"}
	}
	scheme.AddKnownTypes(psmdbSchemeGroupVersion,
		&psmdbv1.PerconaServerMongoDB{}, &psmdbv1.PerconaServerMongoDBList{})

	metav1.AddToGroupVersion(scheme, psmdbSchemeGroupVersion)
	return nil
}

func (r *DatabaseReconciler) addPGKnownTypes(scheme *runtime.Scheme) error {
	pgSchemeGroupVersion := schema.GroupVersion{Group: "pg.percona.com", Version: "v2beta1"}
	scheme.AddKnownTypes(pgSchemeGroupVersion,
		&pgv2beta1.PerconaPGCluster{}, &pgv2beta1.PerconaPGClusterList{})

	metav1.AddToGroupVersion(scheme, pgSchemeGroupVersion)
	return nil
}

func (r *DatabaseReconciler) addPSMDBToScheme(scheme *runtime.Scheme) error {
	builder := runtime.NewSchemeBuilder(r.addPSMDBKnownTypes)
	return builder.AddToScheme(scheme)
}

func (r *DatabaseReconciler) addPXCToScheme(scheme *runtime.Scheme) error {
	builder := runtime.NewSchemeBuilder(r.addPXCKnownTypes)
	return builder.AddToScheme(scheme)
}

func (r *DatabaseReconciler) addPGToScheme(scheme *runtime.Scheme) error {
	builder := runtime.NewSchemeBuilder(r.addPGKnownTypes)
	return builder.AddToScheme(scheme)
}

// SetupWithManager sets up the controller with the Manager.
func (r *DatabaseReconciler) SetupWithManager(mgr ctrl.Manager) error {
	unstructuredResource := &unstructured.Unstructured{}
	unstructuredResource.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "apiextensions.k8s.io",
		Kind:    "CustomResourceDefinition",
		Version: "v1",
	})
	controller := ctrl.NewControllerManagedBy(mgr).
		For(&dbaasv1.DatabaseCluster{})
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
			controller.Owns(&pgv2beta1.PerconaPGCluster{})
		}
	}
	return controller.Complete(r)
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

func (r *DatabaseReconciler) applyTemplate(
	ctx context.Context,
	obj interface{},
	kind string,
	namespacedName types.NamespacedName,
) error {
	unstructuredTemplate := &unstructured.Unstructured{}
	unstructuredDB := &unstructured.Unstructured{}

	unstructuredTemplate.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "dbaas.percona.com",
		Kind:    kind,
		Version: "v1",
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
