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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	everestv1alpha1 "github.com/percona/everest-operator/api/v1alpha1"
)

const (
	cleanupSecretsFinalizer = "percona.com/cleanup-secrets"
	labelBackupStorageName  = "percona.com/backup-storage-name"
)

// BackupStorageReconciler reconciles a BackupStorage object
type BackupStorageReconciler struct {
	client.Client
	Scheme           *runtime.Scheme
	defaultNamespace string
}

//+kubebuilder:rbac:groups=everest.percona.com,resources=backupstorages,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=everest.percona.com,resources=backupstorages/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=everest.percona.com,resources=backupstorages/finalizers,verbs=update

func (r *BackupStorageReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	bs := &everestv1alpha1.BackupStorage{}
	logger := log.FromContext(ctx)
	backupStorageNamespace := req.NamespacedName.Namespace
	if req.NamespacedName.Namespace != r.defaultNamespace {
		backupStorageNamespace = r.defaultNamespace
	}

	err := r.Get(ctx, types.NamespacedName{Name: req.NamespacedName.Name, Namespace: backupStorageNamespace}, bs)
	if err != nil {
		// NotFound cannot be fixed by requeuing so ignore it. During background
		// deletion, we receive delete events from cluster's dependents after
		// cluster is deleted.
		if err = client.IgnoreNotFound(err); err != nil {
			logger.Error(err, "unable to fetch BackupStorage")
		}
		return reconcile.Result{}, err
	}
	if bs.DeletionTimestamp != nil {
		logger.Info("cleaning up the secrets across namespaces")
		fmt.Println(bs.Status.Namespaces)
		for namespace := range bs.Status.Namespaces {
			if namespace == r.defaultNamespace {
				continue
			}
			fmt.Println(namespace)
			secret := &corev1.Secret{}
			err = r.Get(ctx, types.NamespacedName{Name: bs.Name, Namespace: namespace}, secret)
			if err != nil {
				if err = client.IgnoreNotFound(err); err != nil {
					logger.Error(err, "unable to fetch Secret")
				}
				return ctrl.Result{}, err
			}
			err := r.Delete(ctx, secret)
			if err != nil {
				return ctrl.Result{}, err
			}
		}
		logger.Info("all secrets were removed")
		controllerutil.RemoveFinalizer(bs, cleanupSecretsFinalizer)
		if err := r.Update(ctx, bs); err != nil {
			return ctrl.Result{}, err
		}
	}
	defaultSecret := &corev1.Secret{}
	err = r.Get(ctx, types.NamespacedName{Name: req.NamespacedName.Name, Namespace: r.defaultNamespace}, defaultSecret)
	if err != nil {
		if err = client.IgnoreNotFound(err); err != nil {
			logger.Error(err, "unable to fetch Secret")
		}
		return ctrl.Result{}, err
	}
	secret := &corev1.Secret{}
	err = r.Get(ctx, req.NamespacedName, secret)
	if err != nil {
		if err = client.IgnoreNotFound(err); err != nil {
			logger.Error(err, "unable to fetch Secret")
		}
		return ctrl.Result{}, err
	}
	var needsUpdate bool
	if req.NamespacedName.Namespace == r.defaultNamespace {
		logger.Info("Setting controller references for the secret")
		err = controllerutil.SetControllerReference(bs, secret, r.Client.Scheme())
		if err != nil {
			return ctrl.Result{}, err
		}
		if err := r.Update(ctx, secret); err != nil {
			return ctrl.Result{}, err
		}

		if bs.UpdateNamespacesList(req.NamespacedName.Namespace) {
			if err := r.Status().Update(ctx, bs); err != nil {
				return ctrl.Result{}, err
			}

		}
		needsUpdate = true
		if !controllerutil.ContainsFinalizer(bs, cleanupSecretsFinalizer) {
			controllerutil.AddFinalizer(bs, cleanupSecretsFinalizer)
			if err := r.Update(ctx, bs); err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	for namespace := range bs.Status.Namespaces {
		secret := &corev1.Secret{}
		err = r.Get(ctx, types.NamespacedName{Name: secret.Name, Namespace: namespace}, secret)
		if err != nil {
			if err = client.IgnoreNotFound(err); err != nil {
				logger.Error(err, "unable to fetch Secret")
			}
			return ctrl.Result{}, err
		}
		secret.Data = defaultSecret.Data
		secret.Type = defaultSecret.Type
		secret.StringData = defaultSecret.StringData
		err := r.Update(ctx, secret)
		if err != nil {
			return ctrl.Result{}, err
		}
	}
	if needsUpdate {
		err := r.Status().Update(ctx, bs)
		if err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *BackupStorageReconciler) SetupWithManager(mgr ctrl.Manager, defaultNamespace string) error {
	r.defaultNamespace = defaultNamespace
	return ctrl.NewControllerManagedBy(mgr).
		For(&everestv1alpha1.BackupStorage{}).
		Owns(&corev1.Secret{}).
		Complete(r)
}
