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

// Package webhooks ...
//
//nolint:lll
package webhooks

import (
	"context"
	"fmt"
	"regexp"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	fielderrors "k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	everestv1alpha1 "github.com/percona/everest-operator/api/v1alpha1"
	"github.com/percona/everest-operator/internal/consts"
)

// SetupLoadBalancerConfigWebhookWithManager sets up the webhook with the manager.
func SetupLoadBalancerConfigWebhookWithManager(mgr manager.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&everestv1alpha1.LoadBalancerConfig{}).
		WithValidator(&LoadBalancerConfigValidator{
			Client: mgr.GetClient(),
		}).
		Complete()
}

// +kubebuilder:webhook:path=/validate-everest-percona-com-v1alpha1-loadbalancerconfig,mutating=false,failurePolicy=fail,sideEffects=None,groups=everest.percona.com,resources=loadbalancerconfigs,verbs=create;update,versions=v1alpha1,name=vloadbalancerconfig-v1alpha1.everest.percona.com,admissionReviewVersions=v1

// LoadBalancerConfigValidator validates the LoadBalancerConfig resource.
type LoadBalancerConfigValidator struct {
	Client client.Client
}

// ValidateCreate validates the creation of a DatabaseCluster.
func (v *LoadBalancerConfigValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	return nil, v.validateLoadBalancerConfig(ctx, obj)
}

// ValidateUpdate validates the update of a DatabaseCluster.
func (v *LoadBalancerConfigValidator) ValidateUpdate(ctx context.Context, _, newObj runtime.Object) (admission.Warnings, error) {
	return nil, v.validateLoadBalancerConfig(ctx, newObj)
}

// ValidateDelete validates the deletion of a DatabaseCluster.
func (v *LoadBalancerConfigValidator) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

var annotationKeyRegex = regexp.MustCompile(`^(?:(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)*[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?/)?([a-z0-9](?:[a-z0-9_.-]{0,61}[a-z0-9])?)$`) //nolint:lll

func (v *LoadBalancerConfigValidator) validateLoadBalancerConfig(_ context.Context, obj runtime.Object) error {
	lbc, ok := obj.(*everestv1alpha1.LoadBalancerConfig)
	if !ok {
		return fmt.Errorf("expected a LoadBalancerConfig, got %T", obj)
	}

	if !lbc.DeletionTimestamp.IsZero() {
		return nil
	}

	var allErrs []error

	for key := range lbc.Spec.Annotations {
		if !annotationKeyRegex.MatchString(key) {
			allErrs = append(allErrs, fmt.Errorf("invalid annotation key: %s", key))
		}
	}

	if len(allErrs) > 0 {
		return apierrors.NewInvalid(
			everestv1alpha1.GroupVersion.WithKind(consts.LoadBalancerConfigKind).GroupKind(),
			lbc.Name,
			toAggregateFieldErrors(allErrs),
		)
	}

	return nil
}

func toAggregateFieldErrors(errs []error) fielderrors.ErrorList {
	fieldErrList := fielderrors.ErrorList{}
	for _, err := range errs {
		fieldErrList = append(fieldErrList, &fielderrors.Error{
			Type:     fielderrors.ErrorTypeInvalid,
			Field:    "spec.annotations",
			BadValue: err.Error(),
		})
	}

	return fieldErrList
}
