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

	corev1 "k8s.io/api/core/v1"
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
	"github.com/percona/everest-operator/internal/controller/common"
)

// BackupStorageReconciler reconciles a BackupStorage object.
type BackupStorageReconciler struct {
	client.Client
	Scheme *runtime.Scheme
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

	err := r.Get(ctx, types.NamespacedName{Name: req.Name, Namespace: req.Namespace}, bs)
	if err != nil {
		// NotFound cannot be fixed by requeuing so ignore it. During background
		// deletion, we receive delete events from cluster's dependents after
		// cluster is deleted.
		if err = client.IgnoreNotFound(err); err != nil {
			logger.Error(err, "unable to fetch BackupStorage")
		}
		return reconcile.Result{}, err
	}
	defaultSecret := &corev1.Secret{}
	err = r.Get(ctx, types.NamespacedName{
		Name:      req.Name,
		Namespace: req.Namespace,
	},
		defaultSecret)
	if err != nil {
		if err = client.IgnoreNotFound(err); err != nil {
			logger.Error(err, "unable to fetch Secret")
		}
		return ctrl.Result{}, err
	}

	// If the default secret is not owned/controlled by anyone, we will adopt it.
	if controller := metav1.GetControllerOf(defaultSecret); controller == nil {
		logger.Info("setting controller reference for the secret")
		if err := controllerutil.SetControllerReference(bs, defaultSecret, r.Scheme); err != nil {
			return ctrl.Result{}, err
		}
		if err := r.Update(ctx, defaultSecret); err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *BackupStorageReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named("BackupStorage").
		For(&everestv1alpha1.BackupStorage{}).
		Owns(&corev1.Secret{}).
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(r.enqueueBackupStorageForSecret),
			builder.WithPredicates(predicate.ResourceVersionChangedPredicate{}),
		).
		WithEventFilter(common.DefaultNamespaceFilter).
		Watches(
			&corev1.Namespace{},
			common.EnqueueObjectsInNamespace(r.Client, &everestv1alpha1.BackupStorageList{}),
		).
		Complete(r)
}

func (r *BackupStorageReconciler) enqueueBackupStorageForSecret(_ context.Context, obj client.Object) []reconcile.Request {
	secret, ok := obj.(*corev1.Secret)
	if !ok {
		return nil
	}
	backupStorageName, ok := secret.Labels[common.LabelBackupStorageName]
	if !ok {
		return nil
	}
	return []reconcile.Request{{
		NamespacedName: types.NamespacedName{
			Name:      backupStorageName,
			Namespace: secret.GetNamespace(),
		},
	}}
}
