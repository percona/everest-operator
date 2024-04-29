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
	"slices"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	everestv1alpha1 "github.com/percona/everest-operator/api/v1alpha1"
	"github.com/percona/everest-operator/controllers/common"
)

const (
	cleanupSecretsFinalizer = "percona.com/cleanup-secrets" //nolint:gosec
	labelBackupStorageName  = "percona.com/backup-storage-name"
)

// BackupStorageReconciler reconciles a BackupStorage object.
type BackupStorageReconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	systemNamespace string
}

//+kubebuilder:rbac:groups=everest.percona.com,resources=backupstorages,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=everest.percona.com,resources=backupstorages/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=everest.percona.com,resources=backupstorages/finalizers,verbs=update
//+kubebuilder:rbac:groups=core,resources=namespaces,verbs=get;watch;list

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// Modify the Reconcile function to compare the state specified by
// the DatabaseClusterBackup object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.1/pkg/reconcile
func (r *BackupStorageReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	bs := &everestv1alpha1.BackupStorage{}
	logger := log.FromContext(ctx)
	backupStorageNamespace := req.NamespacedName.Namespace
	if req.NamespacedName.Namespace != r.systemNamespace {
		backupStorageNamespace = r.systemNamespace
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
	if bs.GetDeletionTimestamp() != nil {
		logger.Info("cleaning up the secrets across namespaces")
		if err := r.handleDelete(ctx, bs); err != nil {
			return ctrl.Result{}, err
		}
		logger.Info("all secrets were removed")
		controllerutil.RemoveFinalizer(bs, cleanupSecretsFinalizer)
		if err := r.Update(ctx, bs); err != nil {
			return ctrl.Result{}, err
		}
	}
	defaultSecret := &corev1.Secret{}
	err = r.Get(ctx, types.NamespacedName{
		Name:      req.NamespacedName.Name,
		Namespace: r.systemNamespace,
	},
		defaultSecret)
	if err != nil {
		if err = client.IgnoreNotFound(err); err != nil {
			logger.Error(err, "unable to fetch Secret")
		}
		return ctrl.Result{}, err
	}

	if req.NamespacedName.Namespace != r.systemNamespace {
		return ctrl.Result{}, nil
	}

	// Set controllerRef on the defaultSecret.
	if !metav1.IsControlledBy(defaultSecret, bs) {
		logger.Info("setting controller reference for the secret")
		if err := controllerutil.SetControllerReference(bs, defaultSecret, r.Scheme); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.Update(ctx, defaultSecret); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Add finalizer.
	if controllerutil.AddFinalizer(bs, cleanupSecretsFinalizer) {
		if err := r.Update(ctx, bs); err != nil {
			return ctrl.Result{}, err
		}
	}

	needStatusUpdate, err := r.reconcileUsedNamespaces(ctx, bs)
	if err != nil {
		return ctrl.Result{}, err
	}
	if bs.UpdateNamespacesList(defaultSecret.Namespace) || needStatusUpdate {
		if err := r.Status().Update(ctx, bs); err != nil {
			return ctrl.Result{}, err
		}
	}

	logger.Info("updating secrets in used namespaces")
	if err := r.handleSecretUpdate(ctx, bs, defaultSecret); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *BackupStorageReconciler) reconcileUsedNamespaces(ctx context.Context, bs *everestv1alpha1.BackupStorage) (bool, error) {
	// List all secrets with the label backup-storage-name.
	secretList := &corev1.SecretList{}
	err := r.List(ctx, secretList, client.MatchingLabels{labelBackupStorageName: bs.Name})
	if err != nil {
		return false, err
	}
	log := log.FromContext(ctx)
	log.Info("found secrets", "len", len(secretList.Items))
	updated := false
	for _, secret := range secretList.Items {
		if !secret.DeletionTimestamp.IsZero() {
			continue
		}
		val, found := bs.Status.UsedNamespaces[secret.GetNamespace()]
		if !found || !val {
			bs.Status.UsedNamespaces[secret.GetNamespace()] = true
			updated = true
		}
	}
	// Clean-up unused/stale namespaces
	for ns := range bs.Status.UsedNamespaces {
		contains := slices.ContainsFunc(secretList.Items, func(s corev1.Secret) bool {
			return s.GetNamespace() == ns
		})
		if !contains {
			delete(bs.Status.UsedNamespaces, ns)
			updated = true
		}
	}
	return updated, nil
}

func (r *BackupStorageReconciler) handleDelete(ctx context.Context, bs *everestv1alpha1.BackupStorage) error {
	for namespace := range bs.Status.UsedNamespaces {
		if namespace == r.systemNamespace {
			continue
		}
		secret := &corev1.Secret{}
		err := r.Get(ctx, types.NamespacedName{Name: bs.Name, Namespace: namespace}, secret)
		if err != nil {
			if k8serrors.IsNotFound(err) {
				// Already deleted.
				return nil
			}
			return err
		}
		err = r.Delete(ctx, secret)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *BackupStorageReconciler) handleSecretUpdate(ctx context.Context, bs *everestv1alpha1.BackupStorage, defaultSecret *corev1.Secret) error {
	for namespace := range bs.Status.UsedNamespaces {
		if namespace == r.systemNamespace {
			continue
		}
		secret := &corev1.Secret{}
		err := r.Get(ctx, types.NamespacedName{Name: bs.Name, Namespace: namespace}, secret)
		if err != nil {
			return err
		}
		secret.Data = defaultSecret.Data
		secret.Type = defaultSecret.Type
		secret.StringData = defaultSecret.StringData
		err = r.Update(ctx, secret)
		if err != nil {
			return err
		}
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *BackupStorageReconciler) SetupWithManager(mgr ctrl.Manager, systemNamespace string) error {
	r.systemNamespace = systemNamespace
	c := ctrl.NewControllerManagedBy(mgr).
		For(&everestv1alpha1.BackupStorage{}).
		Owns(&corev1.Secret{}).
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(r.enqueueBackupStorageForSecret),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		)
	return c.Complete(r)
}

// given a secret, enqueue a request for its backupstorage (if any)
func (r *BackupStorageReconciler) enqueueBackupStorageForSecret(ctx context.Context, obj client.Object) []reconcile.Request {
	secret, ok := obj.(*corev1.Secret)
	if !ok {
		return nil
	}
	backupStorageName, ok := secret.Labels[common.LabelBackupStorageName]
	return []reconcile.Request{{
		NamespacedName: types.NamespacedName{
			Name:      backupStorageName,
			Namespace: r.systemNamespace,
		},
	}}
}
