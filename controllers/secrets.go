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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	everestv1alpha1 "github.com/percona/everest-operator/api/v1alpha1"
)

// SecretReconciler reconciles a Secret object.
type SecretReconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	systemNamespace string
}

//+kubebuilder:rbac:groups=everest.percona.com,resources=backupstorages,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=everest.percona.com,resources=backupstorages/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=everest.percona.com,resources=backupstorages/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// Modify the Reconcile function to compare the state specified by
// the DatabaseClusterBackup object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.1/pkg/reconcile
func (r *SecretReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	secret := &corev1.Secret{}
	err := r.Get(ctx, req.NamespacedName, secret)
	if err != nil {
		if err = client.IgnoreNotFound(err); err != nil {
			logger.Error(err, "unable to fetch Secret")
		}
		return ctrl.Result{}, err
	}
	if _, ok := secret.Labels[labelBackupStorageName]; ok {
		if err := r.reconcileBackupStorageSecret(ctx, req, secret); err != nil {
			return ctrl.Result{}, err
		}
	}
	if _, ok := secret.Labels[labelMonitoringConfigName]; ok {
		if err := r.reconcileMonitoringConfigSecret(ctx, req, secret); err != nil {
			return ctrl.Result{}, err
		}
	}
	return ctrl.Result{}, nil
}

func (r *SecretReconciler) reconcileMonitoringConfigSecret(ctx context.Context, req ctrl.Request, secret *corev1.Secret) error { //nolint:dupl
	logger := log.FromContext(ctx)
	mc := &everestv1alpha1.MonitoringConfig{}
	err := r.Get(ctx, types.NamespacedName{Name: req.NamespacedName.Name, Namespace: r.systemNamespace}, mc)
	if err != nil {
		// NotFound cannot be fixed by requeuing so ignore it. During background
		// deletion, we receive delete events from cluster's dependents after
		// cluster is deleted.
		if err = client.IgnoreNotFound(err); err != nil {
			logger.Error(err, "unable to fetch BackupStorage")
		}
		return err
	}
	var needsUpdate bool
	if secret.DeletionTimestamp == nil && mc.DeletionTimestamp == nil && mc.UpdateNamespacesList(req.NamespacedName.Namespace) {
		needsUpdate = true
	}
	if secret.DeletionTimestamp != nil && mc.DeletionTimestamp == nil {
		if mc.DeleteUsedNamespace(secret.Namespace) {
			logger.Info("Status updated")
			needsUpdate = true
		}
	}
	if needsUpdate {
		if err := r.Status().Update(ctx, mc); err != nil {
			return err
		}
	}
	return nil
}

func (r *SecretReconciler) reconcileBackupStorageSecret(ctx context.Context, req ctrl.Request, secret *corev1.Secret) error { //nolint:dupl
	logger := log.FromContext(ctx)
	bs := &everestv1alpha1.BackupStorage{}
	err := r.Get(ctx, types.NamespacedName{Name: req.NamespacedName.Name, Namespace: r.systemNamespace}, bs)
	if err != nil {
		// NotFound cannot be fixed by requeuing so ignore it. During background
		// deletion, we receive delete events from cluster's dependents after
		// cluster is deleted.
		if err = client.IgnoreNotFound(err); err != nil {
			logger.Error(err, "unable to fetch BackupStorage")
		}
		return err
	}

	var needsUpdate bool
	if secret.DeletionTimestamp == nil && bs.DeletionTimestamp == nil && bs.UpdateNamespacesList(req.NamespacedName.Namespace) {
		needsUpdate = true
	}
	if secret.DeletionTimestamp != nil && bs.DeletionTimestamp == nil {
		if bs.DeleteUsedNamespace(secret.Namespace) {
			logger.Info("Status updated")
			needsUpdate = true
		}
	}
	if needsUpdate {
		if err := r.Status().Update(ctx, bs); err != nil {
			return err
		}
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *SecretReconciler) SetupWithManager(mgr ctrl.Manager, systemNamespace string) error {
	r.systemNamespace = systemNamespace
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Secret{}).
		WithEventFilter(filterSecretsFunc()).
		Complete(r)
}

func filterSecretsFunc() predicate.Predicate { //nolint:ireturn
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			if _, ok := e.Object.GetLabels()[labelBackupStorageName]; ok {
				return true
			}
			if _, ok := e.Object.GetLabels()[labelMonitoringConfigName]; ok {
				return true
			}
			return false
		},

		UpdateFunc: func(e event.UpdateEvent) bool {
			if _, ok := e.ObjectNew.GetLabels()[labelBackupStorageName]; ok {
				return true
			}
			if _, ok := e.ObjectNew.GetLabels()[labelMonitoringConfigName]; ok {
				return true
			}
			return false
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			if _, ok := e.Object.GetLabels()[labelBackupStorageName]; ok {
				return true
			}
			if _, ok := e.Object.GetLabels()[labelMonitoringConfigName]; ok {
				return true
			}
			return false
		},
	}
}
