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

	"github.com/AlekSi/pointer"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	everestv1alpha1 "github.com/percona/everest-operator/api/v1alpha1"
)

// +kubebuilder:webhook:path=/mutate-everest-percona-com-v1alpha1-databasecluster,mutating=true,failurePolicy=fail,sideEffects=None,groups=everest.percona.com,resources=databaseclusters,verbs=create;update,versions=v1alpha1,name=mdatabasecluster-v1alpha1.everest.percona.com,admissionReviewVersions=v1
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=create;update;get;list
// +kubebuilder:rbac:groups=everest.percona.com,resources=databaseclusters,verbs=get;list;watch;update

// DatabaseClusterDefaulter is a webhook that sets default values for DatabaseCluster resources.
type DatabaseClusterDefaulter struct {
	// Client is the Kubernetes client used to interact with the cluster.
	Client client.Client
}

// Default implements a mutating webhook for DatabaseCluster resources.
func (d *DatabaseClusterDefaulter) Default(ctx context.Context, obj runtime.Object) error {
	db, ok := obj.(*everestv1alpha1.DatabaseCluster)
	if !ok {
		return fmt.Errorf("expected a DatabaseCluster object but got %T", obj)
	}

	log := log.FromContext(ctx).WithName("DatabaseClusterDefaulter").WithValues(
		"name", db.GetName(),
		"namespace", db.GetNamespace(),
	)
	importTpl := pointer.Get(db.Spec.DataSource).DataImport
	err := handleS3CredentialsSecret(ctx, d.Client, db.GetNamespace(), importTpl)
	if err != nil {
		log.Error(err, "handleS3CredentialsSecret failed")
		return err
	}
	return nil
}
