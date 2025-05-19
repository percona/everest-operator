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
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	everestv1alpha1 "github.com/percona/everest-operator/api/v1alpha1"
	"github.com/percona/everest-operator/internal/controller/common"
	"github.com/percona/everest-operator/internal/predicates"
)

// PodSchedulingPolicyReconciler reconciles a PodSchedulingPolicy object.
type PodSchedulingPolicyReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=everest.percona.com,resources=podschedulingpolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=everest.percona.com,resources=podschedulingpolicies/status,verbs=get;update;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// Modify the Reconcile function to compare the state specified by
// the PodSchedulingPolicy object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.1/pkg/reconcile
func (r *PodSchedulingPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (rr ctrl.Result, rerr error) { //nolint:nonamedreturns
	logger := log.FromContext(ctx)
	logger.Info("Reconciling", "request", req)
	defer func() {
		logger.Info("Reconciled", "request", req)
	}()

	psp := &everestv1alpha1.PodSchedulingPolicy{}
	if err := r.Get(ctx, req.NamespacedName, psp); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	pspName := psp.GetName()
	dbList, err := common.DatabaseClustersThatReferenceObject(ctx, r.Client, podSchedulingPolicyNameField, "", pspName)
	if err != nil {
		fetchErr := fmt.Errorf("failed to fetch DB clusters that use pod scheduling policy='%s'", pspName)
		logger.Error(err, fetchErr.Error())
		return ctrl.Result{}, errors.Join(err, fetchErr)
	}

	// Update the status and finalizers of the PodSchedulingPolicy object after the reconciliation.
	defer func() {
		// Nothing to process on delete events
		if !psp.GetDeletionTimestamp().IsZero() {
			return
		}

		psp.Status.InUse = len(dbList.Items) > 0
		psp.Status.LastObservedGeneration = psp.GetGeneration()
		if err = r.Client.Status().Update(ctx, psp); err != nil {
			rr = ctrl.Result{}
			logger.Error(err, fmt.Sprintf("failed to update status for pod scheduling policy='%s'", pspName))
			rerr = errors.Join(err, fmt.Errorf("failed to update status for pod scheduling policy='%s': %w", pspName, err))
		}
	}()

	if err = r.ensureFinalizers(ctx, len(dbList.Items) > 0, psp); err != nil {
		logger.Error(err, fmt.Sprintf("failed to update finalizers for pod scheduling policy='%s'", pspName))
		return ctrl.Result{}, errors.Join(err, fmt.Errorf("failed to update finalizers for pod scheduling policy='%s': %w", pspName, err))
	}

	return ctrl.Result{}, nil
}

func (r *PodSchedulingPolicyReconciler) ensureFinalizers(ctx context.Context, used bool, psp *everestv1alpha1.PodSchedulingPolicy) error {
	var updated bool
	if used {
		updated = controllerutil.AddFinalizer(psp, everestv1alpha1.InUseResourceFinalizer)
	} else {
		updated = controllerutil.RemoveFinalizer(psp, everestv1alpha1.InUseResourceFinalizer)
	}

	if updated {
		return r.Update(ctx, psp)
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *PodSchedulingPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Predicate to trigger reconciliation only on .spec.podSchedulingPolicyName changes in the DatabaseCluster resource.
	dbClusterEventsPredicate := predicate.Funcs{
		// Allow create events only if the .spec.podSchedulingPolicyName is set
		CreateFunc: func(e event.CreateEvent) bool {
			db, ok := e.Object.(*everestv1alpha1.DatabaseCluster)
			if !ok {
				return false
			}
			return db.Spec.PodSchedulingPolicyName != ""
		},

		// Only allow updates when the .spec.podSchedulingPolicyName of the DatabaseCluster resource changes
		UpdateFunc: func(e event.UpdateEvent) bool {
			oldDB, oldOk := e.ObjectOld.(*everestv1alpha1.DatabaseCluster)
			newDB, newOk := e.ObjectNew.(*everestv1alpha1.DatabaseCluster)
			if !oldOk || !newOk {
				return false
			}

			// Trigger reconciliation only if the .spec.podSchedulingPolicyName field has changed
			return oldDB.Spec.PodSchedulingPolicyName != newDB.Spec.PodSchedulingPolicyName
		},

		// Allow delete events only if the .spec.podSchedulingPolicyName is set
		DeleteFunc: func(e event.DeleteEvent) bool {
			db, ok := e.Object.(*everestv1alpha1.DatabaseCluster)
			if !ok {
				return false
			}
			return db.Spec.PodSchedulingPolicyName != ""
		},

		// Nothing to process on generic events
		GenericFunc: func(_ event.GenericEvent) bool {
			return false
		},
	}

	return ctrl.NewControllerManagedBy(mgr).
		Named("PodSchedulingPolicy").
		For(&everestv1alpha1.PodSchedulingPolicy{},
			builder.WithPredicates(predicates.GetPodSchedulingPolicyPredicate(),
				predicate.GenerationChangedPredicate{}),
		).
		// need to watch DBClusters that reference PodSchedulingPolicy to update the policy status.
		Watches(
			&everestv1alpha1.DatabaseCluster{},
			handler.EnqueueRequestsFromMapFunc(func(_ context.Context, obj client.Object) []reconcile.Request {
				db, ok := obj.(*everestv1alpha1.DatabaseCluster)
				if !ok {
					return []reconcile.Request{}
				}

				if db.Spec.PodSchedulingPolicyName == "" {
					// No PodSchedulingPolicyName specified, no need to enqueue
					return []reconcile.Request{}
				}
				return []reconcile.Request{
					{
						NamespacedName: types.NamespacedName{
							Name: db.Spec.PodSchedulingPolicyName,
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
