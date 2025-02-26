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

// Package pxc contains the provider for Percona XtraDB Cluster.
package pxc

import (
	"context"
	"errors"
	"fmt"
	"strings"

	goversion "github.com/hashicorp/go-version"
	pxcv1 "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	everestv1alpha1 "github.com/percona/everest-operator/api/v1alpha1"
	"github.com/percona/everest-operator/internal/controller/common"
	"github.com/percona/everest-operator/internal/controller/providers"
	"github.com/percona/everest-operator/internal/controller/version"
)

const (
	pxcHAProxyEnvSecretName = "haproxy-env-secret" //nolint:gosec // This is not a credential, only a secret name.

	finalizerDeletePXCPodsInOrder = "delete-pxc-pods-in-order"
	finalizerDeletePXCPVC         = "delete-pxc-pvc"
	finalizerDeletePXCSSL         = "delete-ssl"
)

// Provider is a provider for Percona XtraDB Cluster.
type Provider struct {
	providers.ProviderOptions
	*pxcv1.PerconaXtraDBCluster

	// currentPerconaXtraDBClusterSpec holds the current PXC spec.
	currentPerconaXtraDBClusterSpec pxcv1.PerconaXtraDBClusterSpec

	clusterType     common.ClusterType
	operatorVersion *version.Version
}

// New returns a new provider for Percona XtraDB Cluster.
func New(
	ctx context.Context,
	opts providers.ProviderOptions,
) (*Provider, error) {
	pxc := &pxcv1.PerconaXtraDBCluster{}
	client := opts.C
	err := client.Get(
		ctx,
		types.NamespacedName{Name: opts.DB.GetName(), Namespace: opts.DB.GetNamespace()},
		pxc)
	if err != nil && !k8serrors.IsNotFound(err) {
		return nil, err
	}

	dbEngine, err := common.GetDatabaseEngine(ctx, client, common.PXCDeploymentName, opts.DB.GetNamespace())
	if err != nil {
		return nil, err
	}
	opts.DBEngine = dbEngine

	// Get operator version.
	v, err := common.GetOperatorVersion(ctx, opts.C, types.NamespacedName{
		Name:      common.PXCDeploymentName,
		Namespace: opts.DB.GetNamespace(),
	})
	if err != nil {
		return nil, err
	}

	currentSpec := pxc.Spec
	pxc.Spec = defaultSpec()

	p := &Provider{
		PerconaXtraDBCluster:            pxc,
		ProviderOptions:                 opts,
		operatorVersion:                 v,
		currentPerconaXtraDBClusterSpec: currentSpec,
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
	if err := p.handlePauseForPXCRestore(ctx, opts); err != nil {
		return nil, err
	}
	return p, nil
}

// Apply returns the applier for Percona XtraDB Cluster.
//
//nolint:ireturn
func (p *Provider) Apply(ctx context.Context) everestv1alpha1.Applier {
	return &applier{
		Provider: p,
		ctx:      ctx,
	}
}

// During the restore process, the PXC cluster will be paused/unpause by the PXC operator.
// The pause may be overwritten since the Everest operator will also fight for the desired pause state
// set by the user.
// To avoid this race condition, we will explicitly inspect every phase of the restore, and set pause accordingly.
func (p *Provider) handlePauseForPXCRestore(ctx context.Context, opts providers.ProviderOptions) error {
	restores := &pxcv1.PerconaXtraDBClusterRestoreList{}
	if err := opts.C.List(ctx, restores); err != nil {
		return fmt.Errorf("failed to list pxcRestore objects: %w", err)
	}
	if len(restores.Items) == 0 {
		return nil
	}
	for _, restore := range restores.Items {
		// since there is no index for the .spec.pxcCluster field in pxc-restore, the filter is handled manually
		if restore.Spec.PXCCluster != opts.DB.Name {
			continue
		}
		if restore.Status.State == pxcv1.RestoreSucceeded {
			continue
		}
		if restore.Status.State == pxcv1.RestoreStopCluster {
			p.DB.Spec.Paused = true
			return p.C.Update(ctx, p.DB)
		}
		if restore.Status.State == pxcv1.RestoreStartCluster {
			p.DB.Spec.Paused = false
			return p.C.Update(ctx, p.DB)
		}
	}
	return nil
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

	engineSemVer, err := goversion.NewVersion(p.dbEngineVersionOrDefault())
	if err != nil {
		return errors.Join(err, errors.New("cannot parse engine version"))
	}
	engineSemVer = engineSemVer.Core()

	if db.Spec.Engine.Config == "" &&
		engineSemVer.GreaterThanOrEqual(minVersionForOptimizedConfig) {
		switch db.Spec.Engine.Size() {
		case everestv1alpha1.EngineSizeSmall:
			db.Spec.Engine.Config = pxcConfigSizeSmall
		case everestv1alpha1.EngineSizeMedium:
			db.Spec.Engine.Config = pxcConfigSizeMedium
		case everestv1alpha1.EngineSizeLarge:
			db.Spec.Engine.Config = pxcConfigSizeLarge
		}
		updated = true
	}

	if updated {
		return p.C.Update(ctx, db)
	}
	return nil
}

// Status builds the DatabaseCluster Status based on the current state of the Percona XtraDB Cluster.
func (p *Provider) Status(ctx context.Context) (everestv1alpha1.DatabaseClusterStatus, error) {
	status := p.DB.Status
	pxc := p.PerconaXtraDBCluster

	status.Status = everestv1alpha1.AppState(p.PerconaXtraDBCluster.Status.Status).WithCreatingState()
	status.Hostname = pxc.Status.Host
	status.Ready = pxc.Status.Ready
	status.Size = pxc.Status.Size
	status.Message = strings.Join(pxc.Status.Messages, ";")
	status.Port = 3306
	status.CRVersion = pxc.Spec.CRVersion
	status.Details = common.StatusAsPlainTextOrEmptyString(pxc.Status)

	// If a restore is running for this database, set the database status to restoring.
	if restoring, err := common.IsDatabaseClusterRestoreRunning(ctx, p.C, p.DB.GetName(), p.DB.GetNamespace()); err != nil {
		return status, err
	} else if restoring {
		status.Status = everestv1alpha1.AppStateRestoring
	}

	recCRVer, err := common.GetRecommendedCRVersion(ctx, p.C, common.PXCDeploymentName, p.DB)
	if err != nil && !k8serrors.IsNotFound(err) {
		return status, err
	}
	status.RecommendedCRVersion = recCRVer
	return status, nil
}

// Cleanup runs the cleanup routines and returns true if the cleanup is done.
func (p *Provider) Cleanup(ctx context.Context, database *everestv1alpha1.DatabaseCluster) (bool, error) {
	// Even though we no longer set the DBBackupCleanupFinalizer, we still need
	// to handle the cleanup to ensure backward compatibility.
	done, err := common.HandleDBBackupsCleanup(ctx, p.C, database)
	if err != nil || !done {
		return done, err
	}
	return common.HandleUpstreamClusterCleanup(ctx, p.C, database, &pxcv1.PerconaXtraDBCluster{})
}

// DBObject returns the PerconaXtraDBCluster object.
//
//nolint:ireturn
func (p *Provider) DBObject() client.Object {
	p.PerconaXtraDBCluster.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   common.PXCAPIGroup,
		Version: p.operatorVersion.ToK8sVersion(),
		Kind:    common.PerconaXtraDBClusterKind,
	})
	return p.PerconaXtraDBCluster
}
