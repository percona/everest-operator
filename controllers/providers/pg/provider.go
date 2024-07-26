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

// Package pg contains the Percona PostgreSQL provider code.
package pg

import (
	"context"

	pgv2 "github.com/percona/percona-postgresql-operator/pkg/apis/pgv2.percona.com/v2"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	everestv1alpha1 "github.com/percona/everest-operator/api/v1alpha1"
	"github.com/percona/everest-operator/controllers/common"
	"github.com/percona/everest-operator/controllers/providers"
)

const (
	finalizerDeletePGPVC = "percona.com/delete-pvc"
	finalizerDeletePGSSL = "percona.com/delete-ssl"
)

var hostnameAffinity = &corev1.Affinity{
	PodAntiAffinity: &corev1.PodAntiAffinity{
		RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
			{
				TopologyKey: "kubernetes.io/hostname",
			},
		},
	},
}

// Provider is a provider for Percona PostgreSQL.
type Provider struct {
	*pgv2.PerconaPGCluster
	providers.ProviderOptions
	clusterType common.ClusterType
}

// New returns a new provider for Percona PostgreSQL.
func New(
	ctx context.Context,
	opts providers.ProviderOptions,
) (*Provider, error) {
	client := opts.C
	pg := &pgv2.PerconaPGCluster{}
	err := client.Get(ctx, types.NamespacedName{Name: opts.DB.GetName(), Namespace: opts.DB.GetNamespace()}, pg)
	if err != nil && !k8serrors.IsNotFound(err) {
		return nil, err
	}

	dbEngine, err := common.GetDatabaseEngine(ctx, client, common.PGDeploymentName, opts.DB.GetNamespace())
	if err != nil {
		return nil, err
	}
	opts.DBEngine = dbEngine

	pg.Spec = defaultSpec(opts.DB)

	finalizers := []string{
		finalizerDeletePGPVC,
		finalizerDeletePGSSL,
	}
	for _, f := range finalizers {
		controllerutil.AddFinalizer(pg, f)
	}

	p := &Provider{
		PerconaPGCluster: pg,
		ProviderOptions:  opts,
	}
	ct, err := common.GetClusterType(ctx, p.C)
	if err != nil {
		return nil, err
	}
	p.clusterType = ct
	return p, nil
}

// Apply returns the PG applier.
//
//nolint:ireturn
func (p *Provider) Apply(ctx context.Context) everestv1alpha1.Applier {
	return &applier{
		Provider: p,
		ctx:      ctx,
	}
}

// Status builds the DatabaseCluster Status based on the current state of the PerconaPGCluster.
func (p *Provider) Status(ctx context.Context) (everestv1alpha1.DatabaseClusterStatus, error) {
	c := p.C
	pg := p.PerconaPGCluster

	status := p.DB.Status
	status.Status = everestv1alpha1.AppState(pg.Status.State)
	status.Hostname = pg.Status.Host
	status.Ready = pg.Status.Postgres.Ready + pg.Status.PGBouncer.Ready
	status.Size = pg.Status.Postgres.Size + pg.Status.PGBouncer.Size
	status.Port = 5432
	status.CRVersion = pg.Spec.CRVersion

	// If a restore is running for this database, set the database status to restoring
	if restoring, err := common.IsDatabaseClusterRestoreRunning(ctx, c, p.DB.GetName(), p.DB.GetNamespace()); err != nil {
		return status, err
	} else if restoring {
		status.Status = everestv1alpha1.AppStateRestoring
	}

	recCRVer, err := common.GetRecommendedCRVersion(ctx, p.C, common.PGDeploymentName, p.DB)
	if err != nil && !k8serrors.IsNotFound(err) {
		return status, err
	}
	status.RecommendedCRVersion = recCRVer
	return status, nil
}

// Cleanup runs the cleanup routines and returns true if the cleanup is done.
func (p *Provider) Cleanup(ctx context.Context, database *everestv1alpha1.DatabaseCluster) (bool, error) {
	done, err := common.HandleUpstreamClusterCleanup(ctx, p.C, database, &pgv2.PerconaPGCluster{})
	if err != nil || !done {
		return done, err
	}

	// XXX: PGO v2.4.0 has a bug
	// (https://perconadev.atlassian.net/browse/K8SPG-616) where the
	// reconciliation loop gets stuck handling the percona.com/stop-watchers
	// finalizer. In order to work around this issue, we need to wait for the
	// DB to be deleted and then restart the PGO deployment. This is a
	// temporary workaround until the issue is fixed in PGO.
	err = common.RestartDeployment(ctx, p.C, types.NamespacedName{Name: common.PGDeploymentName, Namespace: p.DB.GetNamespace()})
	if err != nil {
		return false, err
	}

	return true, nil
}

// DBObject returns the PerconaPGCluster object.
//
//nolint:ireturn
func (p *Provider) DBObject() client.Object {
	p.PerconaPGCluster.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   common.PGAPIGroup,
		Version: "v2",
		Kind:    common.PerconaPGClusterKind,
	})
	return p.PerconaPGCluster
}
