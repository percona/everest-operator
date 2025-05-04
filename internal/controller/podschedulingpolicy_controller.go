// everest-operator
// Copyright (C) 2025 Percona LLC
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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

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
func (r *PodSchedulingPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (rr ctrl.Result, rerr error) {
	logger := log.FromContext(ctx)
	logger.Info("Reconciling", "request", req)
	defer func() {
		logger.Info("Reconciled", "request", req)
	}()

	psp := &everestv1alpha1.PodSchedulingPolicy{}
	if err := r.Get(ctx, req.NamespacedName, psp); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Handle any necessary cleanup.
	if !psp.GetDeletionTimestamp().IsZero() {
		return ctrl.Result{}, nil
	}

	// Update the status of the PodSchedulingPolicy object after the reconciliation.
	defer func() {
		dbList, err := common.DatabaseClustersThatReferenceObject(ctx, r.Client, podSchedulingPolicyNameField, "", psp.GetName())
		if err != nil {
			rr = ctrl.Result{}
			rerr = errors.Join(err, fmt.Errorf("failed to update status: %w", err))
		}

		psp.Status.Used = len(dbList.Items) > 0
		psp.Status.ObservedGeneration = psp.GetGeneration()
		if err := r.Client.Status().Update(ctx, psp); err != nil {
			rr = ctrl.Result{}
			rerr = errors.Join(err, fmt.Errorf("failed to update status: %w", err))
		}
	}()

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *PodSchedulingPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named("PodSchedulingPolicy").
		For(&everestv1alpha1.PodSchedulingPolicy{},
			// We need to filter out the events that are not in the system namespace,
			// that is why a separate NamespaceFilter predicate is used instead of
			// common.DefaultNamespaceFilter.
			builder.WithPredicates(&predicates.NamespaceFilter{
				AllowNamespaces: []string{common.SystemNamespace},
				GetNamespace: func(ctx context.Context, name string) (*corev1.Namespace, error) {
					namespace := &corev1.Namespace{}
					if err := r.Client.Get(ctx, types.NamespacedName{Name: name}, namespace); err != nil {
						return nil, err
					}
					return namespace, nil
				},
			},
			)).
		Complete(r)
}
