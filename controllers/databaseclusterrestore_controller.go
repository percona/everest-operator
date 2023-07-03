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
	"time"

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

	dbaasv1 "github.com/percona/dbaas-operator/api/v1"
)

const (
	pxcRestoreKind      = "PerconaXtraDBClusterRestore"
	pxcRestoreAPI       = "pxc.percona.com/v1"
	psmdbRestoreKind    = "PerconaServerMongoDBRestore"
	psmdbRestoreAPI     = "psmdb.percona.com/v1"
	psmdbRestoreCRDName = "perconaservermongodbrestores.psmdb.percona.com"
	pxcRestoreCRDName   = "perconaxtradbclusterrestores.pxc.percona.com"
	clusterReadyTimeout = 10 * time.Minute
)

// DatabaseClusterRestoreReconciler reconciles a DatabaseClusterRestore object.
type DatabaseClusterRestoreReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=dbaas.percona.com,resources=databaseclusterrestores,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=dbaas.percona.com,resources=databaseclusterrestores/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=dbaas.percona.com,resources=databaseclusterrestores/finalizers,verbs=update
// +kubebuilder:rbac:groups=pxc.percona.com,resources=perconaxtradbclusterrestores,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=psmdb.percona.com,resources=perconaservermongodbrestores,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *DatabaseClusterRestoreReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("reconciling", "request", req)
	time.Sleep(time.Second)

	cr := &dbaasv1.DatabaseClusterRestore{}
	err := r.Get(ctx, req.NamespacedName, cr)
	if err != nil {
		// NotFound cannot be fixed by requeuing so ignore it. During background
		// deletion, we receive delete events from cluster's dependents after
		// cluster is deleted.
		if err = client.IgnoreNotFound(err); err != nil {
			logger.Error(err, "unable to fetch DatabaseClusterRestore")
		}
		return reconcile.Result{}, err
	}

	if cr.Spec.DatabaseType == dbaasv1.DatabaseEnginePXC {
		if err := r.restorePXC(cr); err != nil { //nolint:contextcheck
			logger.Error(err, "unable to restore PXC Cluster")
			return reconcile.Result{}, err
		}
	}
	if cr.Spec.DatabaseType == dbaasv1.DatabaseEnginePSMDB {
		if err := r.restorePSMDB(cr); err != nil { //nolint:contextcheck
			logger.Error(err, "unable to restore PXC Cluster")
			return reconcile.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *DatabaseClusterRestoreReconciler) ensureClusterIsReady(restore *dbaasv1.DatabaseClusterRestore) error {
	timeoutCtx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	for {
		select {
		case <-timeoutCtx.Done():
			return errors.New("wait timeout exceeded")
		default:
			cluster := &dbaasv1.DatabaseCluster{}
			err := r.Get(context.Background(), types.NamespacedName{Name: restore.Spec.DatabaseCluster, Namespace: restore.Namespace}, cluster)
			if err != nil {
				return err
			}
			if cluster.Status.State == dbaasv1.AppStateReady {
				return nil
			}
		}
	}
}

func (r *DatabaseClusterRestoreReconciler) restorePSMDB(restore *dbaasv1.DatabaseClusterRestore) error {
	if err := r.ensureClusterIsReady(restore); err != nil {
		return err
	}

	psmdbCR := &psmdbv1.PerconaServerMongoDBRestore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      restore.Name,
			Namespace: restore.Namespace,
		},
	}
	if err := controllerutil.SetControllerReference(restore, psmdbCR, r.Client.Scheme()); err != nil {
		return err
	}
	_, err := controllerutil.CreateOrUpdate(context.Background(), r.Client, psmdbCR, func() error {
		psmdbCR.TypeMeta = metav1.TypeMeta{
			APIVersion: psmdbRestoreAPI,
			Kind:       psmdbRestoreKind,
		}
		psmdbCR.Spec.ClusterName = restore.Spec.DatabaseCluster
		if restore.Spec.BackupName == "" && restore.Spec.BackupSource == nil {
			return errors.New("specify either backupName or backupSource")
		}
		if restore.Spec.BackupName != "" {
			psmdbCR.Spec.BackupName = restore.Spec.BackupName
		}
		if restore.Spec.BackupSource != nil {
			psmdbCR.Spec.BackupSource = &psmdbv1.PerconaServerMongoDBBackupStatus{
				Destination: restore.Spec.BackupSource.Destination,
				StorageName: restore.Spec.BackupSource.StorageName,
			}
			switch restore.Spec.BackupSource.StorageType {
			case dbaasv1.BackupStorageS3:
				psmdbCR.Spec.BackupSource.S3 = &psmdbv1.BackupStorageS3Spec{
					Bucket:            restore.Spec.BackupSource.S3.Bucket,
					CredentialsSecret: restore.Spec.BackupSource.S3.CredentialsSecret,
					Region:            restore.Spec.BackupSource.S3.Region,
					EndpointURL:       restore.Spec.BackupSource.S3.EndpointURL,
				}
			case dbaasv1.BackupStorageAzure:
				psmdbCR.Spec.BackupSource.Azure = &psmdbv1.BackupStorageAzureSpec{
					CredentialsSecret: restore.Spec.BackupSource.Azure.CredentialsSecret,
					Container:         restore.Spec.BackupSource.Azure.ContainerName,
					Prefix:            restore.Spec.BackupSource.Azure.Prefix,
				}
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	psmdbCR = &psmdbv1.PerconaServerMongoDBRestore{}
	err = r.Get(context.Background(), types.NamespacedName{Name: restore.Name, Namespace: restore.Namespace}, psmdbCR)
	if err != nil {
		return err
	}
	restore.Status.State = dbaasv1.RestoreState(psmdbCR.Status.State)
	restore.Status.CompletedAt = psmdbCR.Status.CompletedAt
	restore.Status.Message = psmdbCR.Status.Error
	return r.Status().Update(context.Background(), restore)
}

func (r *DatabaseClusterRestoreReconciler) restorePXC(restore *dbaasv1.DatabaseClusterRestore) error {
	pxcCR := &pxcv1.PerconaXtraDBClusterRestore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      restore.Name,
			Namespace: restore.Namespace,
		},
	}
	if err := controllerutil.SetControllerReference(restore, pxcCR, r.Client.Scheme()); err != nil {
		return err
	}
	_, err := controllerutil.CreateOrUpdate(context.Background(), r.Client, pxcCR, func() error {
		pxcCR.TypeMeta = metav1.TypeMeta{
			APIVersion: pxcRestoreAPI,
			Kind:       pxcRestoreKind,
		}
		pxcCR.Spec.PXCCluster = restore.Spec.DatabaseCluster
		if restore.Spec.BackupName == "" && restore.Spec.BackupSource == nil {
			return errors.New("specify either backupName or backupSource")
		}
		if restore.Spec.BackupName != "" {
			pxcCR.Spec.BackupName = restore.Spec.BackupName
		}
		if restore.Spec.BackupSource != nil {
			pxcCR.Spec.BackupSource = &pxcv1.PXCBackupStatus{
				Destination: restore.Spec.BackupSource.Destination,
				StorageName: restore.Spec.BackupSource.StorageName,
			}
			switch restore.Spec.BackupSource.StorageType {
			case dbaasv1.BackupStorageS3:
				pxcCR.Spec.BackupSource.S3 = &pxcv1.BackupStorageS3Spec{
					Bucket:            restore.Spec.BackupSource.S3.Bucket,
					CredentialsSecret: restore.Spec.BackupSource.S3.CredentialsSecret,
					Region:            restore.Spec.BackupSource.S3.Region,
					EndpointURL:       restore.Spec.BackupSource.S3.EndpointURL,
				}
			case dbaasv1.BackupStorageAzure:
				pxcCR.Spec.BackupSource.Azure = &pxcv1.BackupStorageAzureSpec{
					CredentialsSecret: restore.Spec.BackupSource.Azure.CredentialsSecret,
					ContainerPath:     restore.Spec.BackupSource.Azure.ContainerName,
					Endpoint:          restore.Spec.BackupSource.Azure.EndpointURL,
					StorageClass:      restore.Spec.BackupSource.Azure.StorageClass,
				}
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	pxcCR = &pxcv1.PerconaXtraDBClusterRestore{}
	err = r.Get(context.Background(), types.NamespacedName{Name: restore.Name, Namespace: restore.Namespace}, pxcCR)
	if err != nil {
		return err
	}
	restore.Status.State = dbaasv1.RestoreState(pxcCR.Status.State)
	restore.Status.CompletedAt = pxcCR.Status.CompletedAt
	restore.Status.LastScheduled = pxcCR.Status.LastScheduled
	restore.Status.Message = pxcCR.Status.Comments
	return r.Status().Update(context.Background(), restore)
}

func (r *DatabaseClusterRestoreReconciler) addPXCKnownTypes(scheme *runtime.Scheme) error {
	pxcSchemeGroupVersion := schema.GroupVersion{Group: "pxc.percona.com", Version: "v1"}
	scheme.AddKnownTypes(pxcSchemeGroupVersion,
		&pxcv1.PerconaXtraDBClusterRestore{}, &pxcv1.PerconaXtraDBClusterRestoreList{})

	metav1.AddToGroupVersion(scheme, pxcSchemeGroupVersion)
	return nil
}

func (r *DatabaseClusterRestoreReconciler) addPSMDBKnownTypes(scheme *runtime.Scheme) error {
	pxcSchemeGroupVersion := schema.GroupVersion{Group: "psmdb.percona.com", Version: "v1"}
	scheme.AddKnownTypes(pxcSchemeGroupVersion,
		&psmdbv1.PerconaServerMongoDBRestore{}, &psmdbv1.PerconaServerMongoDBRestoreList{})

	metav1.AddToGroupVersion(scheme, pxcSchemeGroupVersion)
	return nil
}

func (r *DatabaseClusterRestoreReconciler) addPXCToScheme(scheme *runtime.Scheme) error {
	builder := runtime.NewSchemeBuilder(r.addPXCKnownTypes)
	return builder.AddToScheme(scheme)
}

func (r *DatabaseClusterRestoreReconciler) addPSMDBToScheme(scheme *runtime.Scheme) error {
	builder := runtime.NewSchemeBuilder(r.addPSMDBKnownTypes)
	return builder.AddToScheme(scheme)
}

// SetupWithManager sets up the controller with the Manager.
func (r *DatabaseClusterRestoreReconciler) SetupWithManager(mgr ctrl.Manager) error {
	unstructuredResource := &unstructured.Unstructured{}
	unstructuredResource.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "apiextensions.k8s.io",
		Kind:    "CustomResourceDefinition",
		Version: "v1",
	})
	controller := ctrl.NewControllerManagedBy(mgr).
		For(&dbaasv1.DatabaseClusterRestore{})
	err := r.Get(context.Background(), types.NamespacedName{Name: pxcRestoreCRDName}, unstructuredResource)
	if err == nil {
		if err := r.addPXCToScheme(r.Scheme); err == nil {
			controller.Owns(&pxcv1.PerconaXtraDBClusterRestore{})
		}
	}
	err = r.Get(context.Background(), types.NamespacedName{Name: psmdbRestoreCRDName}, unstructuredResource)
	if err == nil {
		if err := r.addPSMDBToScheme(r.Scheme); err == nil {
			controller.Owns(&psmdbv1.PerconaServerMongoDBRestore{})
		}
	}
	if err := r.addPSMDBToScheme(r.Scheme); err != nil {
		return err
	}
	if err := mgr.GetFieldIndexer().IndexField(context.TODO(), &dbaasv1.DatabaseClusterRestore{}, "spec.databaseCluster", func(rawObj client.Object) []string {
		res := rawObj.(*dbaasv1.DatabaseClusterRestore) //nolint:forcetypeassert
		return []string{res.Spec.DatabaseCluster}
	}); err != nil {
		return err
	}
	return controller.Complete(r)
}
