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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	everestv1alpha1 "github.com/percona/everest-operator/api/v1alpha1"
	"github.com/percona/everest-operator/controllers/common"
	"github.com/percona/everest-operator/controllers/providers"
)

const (
	pgAPIGroup = "pgv2.percona.com"

	finalizerDeletePGPVC = "percona.com/delete-pvc"
	finalizerDeletePGSSL = "percona.com/delete-ssl"
)

// Provider is a provider for Percona PostgreSQL.
type Provider struct {
	*pgv2.PerconaPGCluster
	providers.ProviderOptions
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

	if err := p.handlePGOperatorVersion(ctx); err != nil {
		return nil, err
	}
	if err := p.handleClusterTypeConfig(ctx); err != nil {
		return nil, err
	}
	if err := p.ensureDataSourceRemoved(ctx, opts.DB); err != nil {
		return nil, err
	}
	return p, nil
}

// DataSource is not needed anymore when db is ready.
// Deleting it to prevent the future restoration conflicts.
func (p *Provider) ensureDataSourceRemoved(ctx context.Context, db *everestv1alpha1.DatabaseCluster) error {
	if db.Status.Status == everestv1alpha1.AppStateReady &&
		db.Spec.DataSource != nil {
		db.Spec.DataSource = nil
		return p.C.Update(ctx, db)
	}
	return nil
}

func (p *Provider) handlePGOperatorVersion(ctx context.Context) error {
	pg := p.PerconaPGCluster
	v, err := common.GetOperatorVersion(ctx, p.C, types.NamespacedName{
		Name:      common.PGDeploymentName,
		Namespace: p.DB.GetNamespace(),
	})
	if err != nil {
		return err
	}
	pg.TypeMeta = metav1.TypeMeta{
		APIVersion: pgAPIGroup + "/v2",
		Kind:       common.PerconaPGClusterKind,
	}
	crVersion := v.ToCRVersion()
	if pg.Spec.CRVersion != "" {
		crVersion = pg.Spec.CRVersion
	}
	pg.Spec.CRVersion = crVersion
	return nil
}

// handleClusterTypeConfig cluster type specific configuration (if any).
func (p *Provider) handleClusterTypeConfig(ctx context.Context) error {
	ct, err := common.GetClusterType(ctx, p.C)
	if err != nil {
		return err
	}
	if ct == common.ClusterTypeEKS {
		affinity := &corev1.Affinity{
			PodAntiAffinity: &corev1.PodAntiAffinity{
				RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{
					{
						TopologyKey: "kubernetes.io/hostname",
					},
				},
			},
		}
		p.PerconaPGCluster.Spec.InstanceSets[0].Affinity = affinity
		p.PerconaPGCluster.Spec.Proxy.PGBouncer.Affinity = affinity
	}
	return nil
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

	// If a restore is running for this database, set the database status to restoring
	if restoring, err := common.IsDatabaseClusterRestoreRunning(ctx, c, p.DB.Spec.Engine.Type, p.DB.GetName(), p.DB.GetNamespace()); err != nil {
		return status, err
	} else if restoring {
		status.Status = everestv1alpha1.AppStateRestoring
	}
	return status, nil
}

// Cleanup runs the cleanup routines and returns true if the cleanup is done.
func (p *Provider) Cleanup(ctx context.Context, database *everestv1alpha1.DatabaseCluster) (bool, error) {
	// Handle cleanup of dbb objects.
	if controllerutil.ContainsFinalizer(database, common.DBCBackupCleanupFinalizer) {
		if done, err := common.DeleteBackupsForDatabase(ctx, p.C, database.GetName(), database.GetNamespace()); err != nil {
			return false, err
		} else if !done {
			return false, nil
		}
	}
	return true, nil
}

// DBObject returns the PerconaPGCluster object.
func (p *Provider) DBObject() client.Object { //nolint:ireturn
	return p.PerconaPGCluster
}
