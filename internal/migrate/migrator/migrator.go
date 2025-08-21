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

// Package migrator contains binary to migrate resources between versions
package migrator

import (
	"context"
	"fmt"
	"os"

	"github.com/go-logr/logr"
	v1 "k8s.io/api/coordination/v1"
	storagev1 "k8s.io/api/storage/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	everestv1alpha1 "github.com/percona/everest-operator/api/v1alpha1"
	"github.com/percona/everest-operator/internal/consts"
	"github.com/percona/everest-operator/internal/controller/common"
)

const (
	systemNamespaceEnvVar              = "SYSTEM_NAMESPACE"
	versionEnvVar                      = "EVEREST_VERSION"
	leaseName                          = "everest-migration-lease"
	lastMigrationVersionAnnotationName = "last-migration-version"
	defaultEKSLoadBalancerConfigName   = "eks-default"
)

// Migrator is the struct representing the migrator.
type Migrator struct {
	l               logr.Logger
	currentVersion  string
	systemNamespace string
	client          client.Client
}

// NewMigrator creates a new migrator.
func NewMigrator(logger logr.Logger) (*Migrator, error) {
	m := &Migrator{
		l:               logger,
		currentVersion:  os.Getenv(versionEnvVar),
		systemNamespace: os.Getenv(systemNamespaceEnvVar),
	}

	cl, err := client.New(config.GetConfigOrDie(), client.Options{Scheme: m.BuildScheme()})
	if err != nil {
		return nil, fmt.Errorf("unable to create client: %w", err)
	}

	m.client = cl

	return m, nil
}

// Migrate function that performs CRs migration.
func (m *Migrator) Migrate(ctx context.Context) (info string, rerr error) { //nolint:nonamedreturns
	lease, err := m.getLease(ctx)
	if err != nil {
		return "", fmt.Errorf("unable to prepare lease: %w", err)
	}

	defer func() {
		if rerr != nil {
			return
		}

		lease.Annotations[lastMigrationVersionAnnotationName] = m.currentVersion

		err = m.client.Update(ctx, lease)
		if err != nil {
			rerr = fmt.Errorf("unable to update lease: %w", err)
		}
	}()

	if lease.Annotations[lastMigrationVersionAnnotationName] == m.currentVersion {
		return "migration is already performed", nil
	}

	info, err = m.migrateResources(ctx)
	if err != nil {
		return "", fmt.Errorf("unable to perform migration: %w", err)
	}

	return info, nil
}

// BuildScheme creates and returns the scheme for k8s client.
func (m *Migrator) BuildScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	utilruntime.Must(storagev1.AddToScheme(scheme))
	utilruntime.Must(everestv1alpha1.AddToScheme(scheme))
	utilruntime.Must(v1.AddToScheme(scheme))

	return scheme
}

func (m *Migrator) getLease(ctx context.Context) (*v1.Lease, error) {
	meta := metav1.ObjectMeta{
		Name:      leaseName,
		Namespace: m.systemNamespace,
	}
	lease := &v1.Lease{
		ObjectMeta: meta,
	}

	err := m.client.Get(ctx, types.NamespacedName{Name: leaseName, Namespace: m.systemNamespace}, lease)
	if k8serrors.IsNotFound(err) {
		lease.Annotations = map[string]string{
			lastMigrationVersionAnnotationName: "",
		}

		err = m.client.Create(ctx, lease)
		if err != nil {
			return nil, err
		}

		err = m.client.Get(ctx, types.NamespacedName{Name: leaseName, Namespace: m.systemNamespace}, lease)
		if err != nil {
			return nil, err
		}

		return lease, nil
	}

	if err != nil {
		return nil, err
	}

	return lease, nil
}

// This is the list of actions to be performed in the operator init container during the migration to the new
// Everest version. Update this method for each next release
//
// For 1.9.0 Everest needs to patch all existing DBs if they are exposed and running on EKS
// to assign the default .spec.proxy.expose.loadBalancerConfigName value.
// Returns error and the additional info if needed.
func (m *Migrator) migrateResources(ctx context.Context) (string, error) {
	clusterType, err := common.GetClusterType(ctx, m.client)
	if err != nil {
		return "", err
	}

	if clusterType != consts.ClusterTypeEKS {
		return "cluster type is not EKS, applying empty migration", nil
	}

	dbs := &everestv1alpha1.DatabaseClusterList{}

	err = m.client.List(ctx, dbs)
	if err != nil {
		return "", err
	}

	for _, db := range dbs.Items {
		if db.Spec.Proxy.Expose.Type == everestv1alpha1.ExposeTypeExternal {
			db.Spec.Proxy.Expose.LoadBalancerConfigName = defaultEKSLoadBalancerConfigName

			err = m.client.Update(ctx, &db)
			if err != nil {
				return "", err
			}
		}
	}

	return "migration is performed successfully", nil
}
