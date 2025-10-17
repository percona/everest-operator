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

// Package enginefeatureseverest contains a set of controller for the enginefeatures.everest.percona.com API group
package enginefeatureseverest

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	enginefeatureseverestv1alpha1 "github.com/percona/everest-operator/api/enginefeatures.everest/v1alpha1"
	everestv1alpha1 "github.com/percona/everest-operator/api/everest/v1alpha1"
	everestcontrollers "github.com/percona/everest-operator/internal/controller/everest"
	"github.com/percona/everest-operator/internal/controller/everest/common"
	enginefeaturespredicate "github.com/percona/everest-operator/internal/predicates/enginefeatures"
)

// SplitHorizonDNSConfigReconciler reconciles a SplitHorizonDNSConfig object.
type SplitHorizonDNSConfigReconciler struct {
	client.Client

	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=enginefeatures.everest.percona.com,resources=splithorizondnsconfigs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=enginefeatures.everest.percona.com,resources=splithorizondnsconfigs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=enginefeatures.everest.percona.com,resources=splithorizondnsconfigs/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the SplitHorizonDNSConfig object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.22.1/pkg/reconcile
func (r *SplitHorizonDNSConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := logf.FromContext(ctx)
	logger.Info("Reconciling")
	defer func() {
		logger.Info("Reconciled")
	}()

	shdc := &enginefeatureseverestv1alpha1.SplitHorizonDNSConfig{}
	if err := r.Get(ctx, req.NamespacedName, shdc); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	dbList, err := common.DatabaseClustersThatReferenceObject(ctx, r.Client,
		everestcontrollers.SplitHorizonDNSConfigNameField,
		shdc.GetNamespace(),
		shdc.GetName())
	if err != nil {
		msg := fmt.Sprintf("failed to fetch DB clusters that use split-horizon dns config='%s/%s'", shdc.GetNamespace(), shdc.GetName())
		logger.Error(err, msg)
		return ctrl.Result{}, fmt.Errorf("%s: %w", msg, err)
	}

	// Update the status of the SplitHorizonDNSConfig object after the reconciliation.
	defer func() {
		// Nothing to process once resource is going to be deleted.
		if !shdc.GetDeletionTimestamp().IsZero() {
			return
		}

		shdc.Status.InUse = len(dbList.Items) > 0
		shdc.Status.LastObservedGeneration = shdc.GetGeneration()
		if err = r.Client.Status().Update(ctx, shdc); err != nil {
			msg := fmt.Sprintf("failed to update status for split-horizon dns config='%s/%s'", shdc.GetNamespace(), shdc.GetName())
			logger.Error(err, msg)
		}
	}()

	if err = common.EnsureInUseFinalizer(ctx, r.Client, len(dbList.Items) > 0, shdc); err != nil {
		msg := fmt.Sprintf("failed to update finalizers for split-horizon dns config='%s/%s'", shdc.GetNamespace(), shdc.GetName())
		logger.Error(err, msg)
		return ctrl.Result{}, fmt.Errorf("%s: %w", msg, err)
	}

	if shdc.Spec.TLS.Certificate == nil {
		// Nothing to do.
		return ctrl.Result{}, nil
	}

	// Move TLS certificate from the spec to a secret if needed.
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      shdc.Spec.TLS.SecretName,
			Namespace: shdc.GetNamespace(),
		},
	}

	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, secret, func() error {
		secret.Data = map[string][]byte{
			"tls.crt": []byte(shdc.Spec.TLS.Certificate.TLSCert),
			"tls.key": []byte(shdc.Spec.TLS.Certificate.TLSKey),
			"ca.crt":  []byte(shdc.Spec.TLS.Certificate.CACert),
		}
		secret.Type = corev1.SecretTypeTLS

		if secret.GetUID() == "" {
			// Set owner reference only when the secret is created.
			// There may be a case when the secret already exists and is not owned by us.
			if err := controllerutil.SetControllerReference(shdc, secret, r.Scheme); err != nil {
				return fmt.Errorf("failed to set controller reference for secret=%s/%s: %w",
					secret.GetNamespace(), secret.GetName(), err)
			}
		}
		return nil
	}); err != nil {
		return ctrl.Result{}, err
	}

	// Reset Certificate in the spec to avoid storing sensitive data in the CR.
	shdc.Spec.TLS.Certificate = nil
	if err = r.Update(ctx, shdc); err != nil {
		msg := fmt.Sprintf("failed to reset certificate in the spec for split-horizon dns config='%s/%s'",
			shdc.GetNamespace(), shdc.GetName())
		logger.Error(err, msg)
		return ctrl.Result{}, fmt.Errorf("%s: %w", msg, err)
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *SplitHorizonDNSConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Predicate to trigger reconciliation only on .spec.engineFeatures.psmdb.splitHorizonDnsConfigName changes in the DatabaseCluster resource.
	dbClusterEventsPredicate := predicate.Funcs{
		// Allow create events only if the .spec.engineFeatures.psmdb.splitHorizonDnsConfigName is set
		CreateFunc: func(e event.CreateEvent) bool {
			db, ok := e.Object.(*everestv1alpha1.DatabaseCluster)
			if !ok {
				return false
			}

			return common.GetSplitHorizonDNSConfigNameFromDB(db) != ""
		},

		// Only allow updates when the .spec.engineFeatures.psmdb.splitHorizonDnsConfigName of the DatabaseCluster resource changes
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldDB, oldOk := e.ObjectOld.(*everestv1alpha1.DatabaseCluster)
			newDB, newOk := e.ObjectNew.(*everestv1alpha1.DatabaseCluster)
			if !oldOk || !newOk {
				return false
			}

			// Trigger reconciliation only if the .spec.engineFeatures.psmdb.splitHorizonDnsConfigName field has changed
			return common.GetSplitHorizonDNSConfigNameFromDB(oldDB) !=
				common.GetSplitHorizonDNSConfigNameFromDB(newDB)
		},

		// Allow delete events only if the .spec.engineFeatures.psmdb.splitHorizonDnsConfigName is set
		DeleteFunc: func(e event.DeleteEvent) bool {
			db, ok := e.Object.(*everestv1alpha1.DatabaseCluster)
			if !ok {
				return false
			}
			return common.GetSplitHorizonDNSConfigNameFromDB(db) != ""
		},

		// Nothing to process on generic events
		GenericFunc: func(_ event.GenericEvent) bool {
			return false
		},
	}

	return ctrl.NewControllerManagedBy(mgr).
		Named("splithorizondnsconfig").
		For(&enginefeatureseverestv1alpha1.SplitHorizonDNSConfig{},
			builder.WithPredicates(enginefeaturespredicate.GetSplitHorizonDNSConfigPredicate(),
				predicate.GenerationChangedPredicate{},
				common.DefaultNamespaceFilter),
		).
		// need to watch DBClusters that reference SplitHorizonDNSConfig to update the config status.
		Watches(
			&everestv1alpha1.DatabaseCluster{},
			handler.EnqueueRequestsFromMapFunc(func(_ context.Context, obj client.Object) []reconcile.Request {
				db, ok := obj.(*everestv1alpha1.DatabaseCluster)
				if !ok {
					return []reconcile.Request{}
				}

				shdcName := common.GetSplitHorizonDNSConfigNameFromDB(db)
				if shdcName == "" {
					// No SplitHorizonDNSConfig specified, no need to enqueue
					return []reconcile.Request{}
				}

				// Enqueue the referenced SplitHorizonDNSConfig for reconciliation
				return []reconcile.Request{
					{
						NamespacedName: types.NamespacedName{
							Name:      shdcName,
							Namespace: db.GetNamespace(),
						},
					},
				}
			}),
			builder.WithPredicates(predicate.GenerationChangedPredicate{},
				dbClusterEventsPredicate,
				common.DefaultNamespaceFilter),
		).
		Complete(r)
}
