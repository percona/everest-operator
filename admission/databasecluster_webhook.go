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

package admission

import (
	"context"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	everestv1alpha1 "github.com/percona/everest-operator/api/v1alpha1"
)

type DatabaseClusterWebhook struct {
	client client.Client
}

// log is for logging in this package.
var databaseclusterlog = logf.Log.WithName("databasecluster-resource")

func (r *DatabaseClusterWebhook) SetupWebhookWithManager(mgr ctrl.Manager) error {
	r.client = mgr.GetClient()
	return ctrl.NewWebhookManagedBy(mgr).
		For(&everestv1alpha1.DatabaseCluster{}).
		WithValidator(r).
		Complete()
}

var _ webhook.CustomValidator = &DatabaseClusterWebhook{}

//+kubebuilder:webhook:path=/validate-everest-percona-com-v1alpha1-databasecluster,mutating=false,failurePolicy=fail,sideEffects=None,groups=everest.percona.com,resources=databaseclusters,verbs=create;update,versions=v1alpha1,name=vdatabasecluster.kb.io,admissionReviewVersions=v1

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *DatabaseClusterWebhook) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	databaseCluster := obj.(*everestv1alpha1.DatabaseCluster)
	databaseclusterLog.Info("validate create", "name", databaseCluster.Name)
	var warnings admission.Warnings

	return warnings, nil
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *DatabaseClusterWebhook) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	databaseCluster := newObj.(*everestv1alpha1.DatabaseCluster)
	databaseclusterLog.Info("validate create", "name", databaseCluster.Name)

	var warnings admission.Warnings

	return warnings, nil
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *DatabaseClusterWebhook) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	databaseCluster := obj.(*everestv1alpha1.DatabaseCluster)
	databaseclusterLog.Info("validate create", "name", databaseCluster.Name)

	var warnings admission.Warnings
	return warnings, nil
}
