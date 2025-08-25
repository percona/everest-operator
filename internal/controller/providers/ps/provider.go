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

// Package ps contains the provider for Percona Server for MySQL Cluster.
package ps

import (
	"context"

	psv1 "github.com/percona/percona-server-mysql-operator/api/v1alpha1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	everestv1alpha1 "github.com/percona/everest-operator/api/v1alpha1"
	"github.com/percona/everest-operator/internal/consts"
	"github.com/percona/everest-operator/internal/controller/common"
	"github.com/percona/everest-operator/internal/controller/providers"
	"github.com/percona/everest-operator/internal/controller/version"
)

const (
	psHAProxyEnvSecretName = "ps-haproxy-env-secret" //nolint:gosec // This is not a credential, only a secret name.

	finalizerDeletePSPodsInOrder = "delete-mysql-pods-in-order"
	finalizerDeletePSPVC         = "delete-mysql-pvc"
	finalizerDeletePSSSL         = "delete-ssl"
)

// Provider is a provider for Percona Server for MySQL Cluster.
type Provider struct {
	providers.ProviderOptions
	*psv1.PerconaServerMySQL

	// currentPSSpec holds the current PS spec.
	currentPSSpec psv1.PerconaServerMySQLSpec

	clusterType     consts.ClusterType
	operatorVersion *version.Version
}

// New returns a new provider for Percona Server for MySQL Cluster.
func New(
	ctx context.Context,
	opts providers.ProviderOptions,
) (*Provider, error) {
	ps := &psv1.PerconaServerMySQL{}
	err := opts.C.Get(
		ctx,
		types.NamespacedName{Name: opts.DB.GetName(), Namespace: opts.DB.GetNamespace()},
		ps)
	if err != nil && !k8serrors.IsNotFound(err) {
		return nil, err
	}

	dbEngine, err := common.GetDatabaseEngine(ctx, opts.C, consts.PSDeploymentName, opts.DB.GetNamespace())
	if err != nil {
		return nil, err
	}
	opts.DBEngine = dbEngine

	// Get operator version.
	// v, err := common.GetOperatorVersion(ctx, opts.C, types.NamespacedName{
	// 	Name:      consts.PSDeploymentName,
	// 	Namespace: opts.DB.GetNamespace(),
	// })

	// TODO: Switch all providers read dbEngine.Status.OperatorVersion.
	v, err := version.NewVersion(dbEngine.Status.OperatorVersion)
	if err != nil {
		return nil, err
	}

	currentSpec := ps.Spec
	ps.Spec = defaultSpec()

	p := &Provider{
		PerconaServerMySQL: ps,
		ProviderOptions:    opts,
		operatorVersion:    v,
		currentPSSpec:      currentSpec,
	}

	// Get cluster type.
	ct, err := common.GetClusterType(ctx, p.C)
	if err != nil {
		return nil, err
	}
	p.clusterType = ct

	if err := p.ensureDefaults(ctx); err != nil {
		return nil, err
	}
	return p, nil
}

// Apply returns the applier for Percona Server for MySQL Cluster.
//
//nolint:ireturn
func (p *Provider) Apply(ctx context.Context) everestv1alpha1.Applier {
	return &applier{
		Provider: p,
		ctx:      ctx,
	}
}

func (p *Provider) dbEngineVersionOrDefault() string {
	engineVersion := p.DB.Spec.Engine.Version
	if engineVersion == "" {
		engineVersion = p.DBEngine.BestEngineVersion()
	}
	return engineVersion
}

func (p *Provider) ensureDefaults(ctx context.Context) error {
	db := p.DB
	updated := false
	if db.Spec.Proxy.Type == "" {
		db.Spec.Proxy.Type = everestv1alpha1.ProxyTypeHAProxy
		updated = true
	}

	// FIXME: Determine PS config for each cluster size.
	// if db.Spec.Engine.Config == "" {
	// 	switch db.Spec.Engine.Size() {
	// 	case everestv1alpha1.EngineSizeSmall:
	// 		db.Spec.Engine.Config = pxcConfigSizeSmall
	// 	case everestv1alpha1.EngineSizeMedium:
	// 		db.Spec.Engine.Config = pxcConfigSizeMedium
	// 	case everestv1alpha1.EngineSizeLarge:
	// 		db.Spec.Engine.Config = pxcConfigSizeLarge
	// 	}
	// 	updated = true
	// }

	if updated {
		return p.C.Update(ctx, db)
	}
	return nil
}

// Status builds the DatabaseCluster Status based on the current state of the Percona Server for MySQL Cluster.
func (p *Provider) Status(ctx context.Context) (everestv1alpha1.DatabaseClusterStatus, error) {
	status := p.DB.Status
	ps := p.PerconaServerMySQL

	status.Status = everestv1alpha1.AppState(ps.Status.State).WithCreatingState()
	status.Hostname = ps.Status.Host
	status.Ready = ps.Status.MySQL.Ready
	// status.Size = ps.Status.Size
	// status.Message = strings.Join(ps.Status.Messages, ";")
	status.Port = 3306
	status.CRVersion = ps.Spec.CRVersion
	status.Details = common.StatusAsPlainTextOrEmptyString(ps.Status)

	// If a restore is running for this database, set the database status to restoring.
	if restoring, err := common.IsDatabaseClusterRestoreRunning(ctx, p.C, p.DB.GetName(), p.DB.GetNamespace()); err != nil {
		return status, err
	} else if restoring {
		status.Status = everestv1alpha1.AppStateRestoring
	}

	// If the current version of the database is different from the version in
	// the CR, an upgrade is pending or in progress.
	if p.DB.Spec.Engine.Version != "" && ps.Status.MySQL.Version != "" && p.DB.Spec.Engine.Version != ps.Status.MySQL.Version {
		status.Status = everestv1alpha1.AppStateUpgrading
	}

	recCRVer, err := common.GetRecommendedCRVersion(ctx, p.C, consts.PSDeploymentName, p.DB)
	if err != nil && !k8serrors.IsNotFound(err) {
		return status, err
	}
	status.RecommendedCRVersion = recCRVer

	return status, nil
}

// when a PXC restore is in progress, we will retry reconciliation
// after the specified duration.
// const defaultRestoreRequeueDuration = 15 * time.Second

// RunPreReconcileHook runs the pre-reconcile hook for the PS provider.
func (p *Provider) RunPreReconcileHook(_ context.Context) (providers.HookResult, error) {
	// The pxc-operator does some funny things to the PXC spec during a restore.
	// We must avoid interfering with that process, so we simply skip reconciliation.
	// Replicating the same behavior here would be a nightmare, so its simpler to just do this.

	// TODO: Uncomment this if PS-operator does the same as PXC-operator.
	// if ok, err := common.IsDatabaseClusterRestoreRunning(ctx, p.C, p.DB.GetName(), p.DB.GetNamespace()); err != nil {
	// 	return providers.HookResult{}, err
	// } else if ok {
	// 	return providers.HookResult{
	// 		RequeueAfter: defaultRestoreRequeueDuration,
	// 		Message:      "Restore is in progress",
	// 	}, nil
	// }
	return providers.HookResult{}, nil
}

// Cleanup runs the cleanup routines and returns true if the cleanup is done.
func (p *Provider) Cleanup(ctx context.Context, database *everestv1alpha1.DatabaseCluster) (bool, error) {
	return common.HandleUpstreamClusterCleanup(ctx, p.C, database, &psv1.PerconaServerMySQL{})
}

// DBObject returns the PerconaServerMySQL object.
//
//nolint:ireturn
func (p *Provider) DBObject() client.Object {
	p.PerconaServerMySQL.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   consts.PSAPIGroup,
		Version: p.operatorVersion.ToK8sVersion(),
		Kind:    consts.PerconaServerMySQLKind,
	})
	return p.PerconaServerMySQL
}
