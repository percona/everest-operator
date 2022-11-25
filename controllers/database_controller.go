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

package controllers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/AlekSi/pointer"
	psmdbAPIs "github.com/percona/percona-server-mongodb-operator/pkg/apis"
	psmdbv1 "github.com/percona/percona-server-mongodb-operator/pkg/apis/psmdb/v1"
	pxcAPIs "github.com/percona/percona-xtradb-cluster-operator/pkg/apis"

	pxcv1 "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
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

	dbaasv1 "github.com/percona/dbaas-operator/api/v1"
)

const (
	PerconaXtraDBClusterKind = "PerconaXtraDBCluster"
	PerconaServerMongoDBKind = "PerconaServerMongoDB"
	pxcDeploymentName        = "percona-xtradb-cluster-operator"
	psmdbDeploymentName      = "percona-server-mongodb-operator"
	pxcCRDName               = "perconaservermongodbs.psmdb.percona.com"
	psmdbCRDName             = "perconaxtradbclusters.pxc.percona.com"
	pxcAPIGroup              = "pxc.percona.com"
	psmdbAPIGroup            = "psmdb.percona.com"
	haProxyTemplate          = "percona/percona-xtradb-cluster-operator:%s-haproxy"
)

// DatabaseReconciler reconciles a Database object
type DatabaseReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=dbaas.percona.com,resources=databaseclusters,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=dbaas.percona.com,resources=databaseclusters/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=dbaas.percona.com,resources=databaseclusters/finalizers,verbs=update

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
	if database.Spec.Database == "pxc" {
		err := r.reconcilePXC(ctx, req, database)
		return reconcile.Result{}, err
	}
	if database.Spec.Database == "psmdb" {
		err := r.reconcilePSMDB(ctx, req, database)
		return reconcile.Result{}, err
	}
	return reconcile.Result{RequeueAfter: 5 * time.Second}, nil
}
func (r *DatabaseReconciler) reconcilePSMDB(ctx context.Context, req ctrl.Request, database *dbaasv1.DatabaseCluster) error {
	diskSize, err := resource.ParseQuantity(database.Spec.DBInstance.DiskSize)
	if err != nil {
		return err
	}
	memory, err := resource.ParseQuantity(database.Spec.DBInstance.Memory)
	if err != nil {
		return err
	}
	cpu, err := resource.ParseQuantity(database.Spec.DBInstance.CPU)
	if err != nil {
		return err
	}
	version, err := r.getOperatorVersion(ctx, types.NamespacedName{
		Namespace: req.NamespacedName.Namespace,
		Name:      pxcDeploymentName,
	})
	if err != nil {
		return err
	}
	psmdb := &psmdbv1.PerconaServerMongoDB{
		ObjectMeta: metav1.ObjectMeta{
			Name:       database.Name,
			Namespace:  database.Namespace,
			Finalizers: []string{"delete-psmdb-pvc"},
		},
	}
	if err := controllerutil.SetControllerReference(database, psmdb, r.Client.Scheme()); err != nil {
		return err
	}
	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, psmdb, func() error {
		psmdb.TypeMeta = metav1.TypeMeta{
			APIVersion: version.ToAPIVersion(psmdbAPIGroup),
			Kind:       PerconaServerMongoDBKind,
		}
		psmdb.Spec = psmdbv1.PerconaServerMongoDBSpec{
			CRVersion:  version.ToCRVersion(),
			UnsafeConf: true,
			Image:      database.Spec.DatabaseImage,
			Secrets: &psmdbv1.SecretsSpec{
				Users: database.Spec.SecretsName,
			},
			UpgradeOptions: psmdbv1.UpgradeOptions{ // TODO: Get rid of hardcode
				Apply:    "disabled",
				Schedule: "0 4 * * *",
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
					EncryptionKeySecret:  fmt.Sprintf("%s-mongodb-encryption-key", database.Name),
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
					Name:          "rs0",
					Configuration: psmdbv1.MongoConfiguration(database.Spec.DatabaseConfig),
					Size:          database.Spec.ClusterSize,
					VolumeSpec: &psmdbv1.VolumeSpec{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimSpec{
							StorageClassName: database.Spec.DBInstance.StorageClassName,
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceStorage: diskSize,
								},
							},
						},
					},
					// TODO: Add pod disruption budget
					MultiAZ: psmdbv1.MultiAZ{
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    cpu,
								corev1.ResourceMemory: memory,
							},
						},
					},
				},
			},
		}
		if database.Spec.ClusterSize != 1 {
			psmdb.Spec.Sharding = psmdbv1.Sharding{
				Enabled: true,
				ConfigsvrReplSet: &psmdbv1.ReplsetSpec{
					Size: database.Spec.ClusterSize,
					VolumeSpec: &psmdbv1.VolumeSpec{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimSpec{
							StorageClassName: database.Spec.DBInstance.StorageClassName,
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceStorage: diskSize,
								},
							},
						},
					},
					Arbiter: psmdbv1.Arbiter{
						Enabled: false,
					},
				},
				Mongos: &psmdbv1.MongosSpec{
					Size: database.Spec.LoadBalancer.Size,
					Expose: psmdbv1.MongosExpose{
						LoadBalancerSourceRanges: database.Spec.LoadBalancer.LoadBalancerSourceRanges,
						ServiceAnnotations:       database.Spec.LoadBalancer.Annotations,
					},
					Configuration: psmdbv1.MongoConfiguration(database.Spec.LoadBalancer.Configuration),
					MultiAZ: psmdbv1.MultiAZ{
						Resources: database.Spec.LoadBalancer.Resources,
					},
					// TODO: Add traffic policy
				},
			}
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
	if err := r.Status().Update(ctx, database); err != nil {
		return err
	}
	return nil
}
func (r *DatabaseReconciler) reconcilePXC(ctx context.Context, req ctrl.Request, database *dbaasv1.DatabaseCluster) error {
	diskSize, err := resource.ParseQuantity(database.Spec.DBInstance.DiskSize)
	if err != nil {
		return err
	}
	memory, err := resource.ParseQuantity(database.Spec.DBInstance.Memory)
	if err != nil {
		return err
	}
	cpu, err := resource.ParseQuantity(database.Spec.DBInstance.CPU)
	if err != nil {
		return err
	}
	version, err := r.getOperatorVersion(ctx, types.NamespacedName{
		Namespace: req.NamespacedName.Namespace,
		Name:      pxcDeploymentName,
	})
	if err != nil {
		return err
	}
	pxc := &pxcv1.PerconaXtraDBCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:       database.Name,
			Namespace:  database.Namespace,
			Finalizers: []string{"delete-proxysql-pvc", "delete-pxc-pvc"},
		},
	}
	if err := controllerutil.SetControllerReference(database, pxc, r.Client.Scheme()); err != nil {
		return err
	}
	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, pxc, func() error {
		pxc.TypeMeta = metav1.TypeMeta{
			APIVersion: version.ToAPIVersion(pxcAPIGroup),
			Kind:       PerconaXtraDBClusterKind,
		}
		pxc.Spec = pxcv1.PerconaXtraDBClusterSpec{
			CRVersion:         version.ToCRVersion(),
			AllowUnsafeConfig: true,
			Pause:             database.Spec.Pause,
			SecretsName:       database.Spec.SecretsName,
			UpgradeOptions: pxcv1.UpgradeOptions{ // TODO: Get rid of hardcode
				Apply:    "8.0-recommended",
				Schedule: "0 4 * * *",
			},
			PXC: &pxcv1.PXCSpec{
				PodSpec: &pxcv1.PodSpec{
					Configuration: database.Spec.DatabaseConfig,
					ServiceType:   corev1.ServiceTypeClusterIP,
					Size:          database.Spec.ClusterSize,
					Image:         database.Spec.DatabaseImage,
					VolumeSpec: &pxcv1.VolumeSpec{
						PersistentVolumeClaim: &corev1.PersistentVolumeClaimSpec{
							StorageClassName: database.Spec.DBInstance.StorageClassName,
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceStorage: diskSize,
								},
							},
						},
					},
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    cpu,
							corev1.ResourceMemory: memory,
						},
					},
				},
			},
		}
		if database.Spec.LoadBalancer.Type == "haproxy" {
			if database.Spec.LoadBalancer.Image == "" {
				database.Spec.LoadBalancer.Image = fmt.Sprintf(haProxyTemplate, version.String())

			}
			pxc.Spec.HAProxy = &pxcv1.HAProxySpec{
				PodSpec: pxcv1.PodSpec{
					Size:                     database.Spec.LoadBalancer.Size,
					ServiceType:              database.Spec.LoadBalancer.ExposeType,
					Configuration:            database.Spec.LoadBalancer.Configuration,
					LoadBalancerSourceRanges: database.Spec.LoadBalancer.LoadBalancerSourceRanges,
					Annotations:              database.Spec.LoadBalancer.Annotations,
					ExternalTrafficPolicy:    database.Spec.LoadBalancer.TrafficPolicy,
					Resources:                database.Spec.LoadBalancer.Resources,
					Enabled:                  true,
					Image:                    database.Spec.LoadBalancer.Image,
				},
			}
		}
		if database.Spec.LoadBalancer.Type == "proxysql" {
			pxc.Spec.ProxySQL = &pxcv1.PodSpec{
				Size:                     database.Spec.LoadBalancer.Size,
				ServiceType:              database.Spec.LoadBalancer.ExposeType,
				Configuration:            database.Spec.LoadBalancer.Configuration,
				LoadBalancerSourceRanges: database.Spec.LoadBalancer.LoadBalancerSourceRanges,
				Annotations:              database.Spec.LoadBalancer.Annotations,
				ExternalTrafficPolicy:    database.Spec.LoadBalancer.TrafficPolicy,
				Resources:                database.Spec.LoadBalancer.Resources,
				Enabled:                  true,
				Image:                    database.Spec.LoadBalancer.Image,
			}
		}
		if database.Spec.Restart && !pxc.Spec.Pause {
			pxc.Spec.Pause = true
		}
		if database.Spec.Restart && pxc.Status.Status == pxcv1.AppStatePaused {
			pxc.Spec.Pause = false
			database.Spec.Restart = false
			r.Update(ctx, database)
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
	if err := r.Status().Update(ctx, database); err != nil {
		return err
	}
	return nil
}
func (r *DatabaseReconciler) getOperatorVersion(ctx context.Context, name types.NamespacedName) (*Version, error) {
	deployment := &appsv1.Deployment{}
	if err := r.Get(ctx, name, deployment); err != nil {
		return nil, err
	}
	version := strings.Split(deployment.Spec.Template.Spec.Containers[0].Image, ":")[1]
	return NewVersion(version)
}

// SetupWithManager sets up the controller with the Manager.
func (r *DatabaseReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if err := pxcAPIs.AddToScheme(r.Scheme); err != nil {
		return err
	}
	if err := psmdbAPIs.AddToScheme(r.Scheme); err != nil {
		return err
	}
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
		controller.Owns(&pxcv1.PerconaXtraDBCluster{})
		fmt.Println("Registered PXC")
	}
	err = r.Get(context.Background(), types.NamespacedName{Name: psmdbCRDName}, unstructuredResource)
	if err == nil {
		controller.Owns(&pxcv1.PerconaXtraDBCluster{})
		fmt.Println("Registered psmdb")
	}
	return controller.Complete(r)
}
