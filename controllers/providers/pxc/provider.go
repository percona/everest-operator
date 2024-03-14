package pxc

import (
	"context"
	"strings"

	"github.com/AlekSi/pointer"
	everestv1alpha1 "github.com/percona/everest-operator/api/v1alpha1"
	"github.com/percona/everest-operator/controllers/common"
	"github.com/percona/everest-operator/controllers/providers"
	pxcv1 "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	pxcHAProxyEnvSecretName = "haproxy-env-secret" //nolint:gosec // This is not a credential, only a secret name.

	finalizerDeletePXCPodsInOrder = "delete-pxc-pods-in-order"
	finalizerDeletePXCPVC         = "delete-pxc-pvc"
	finalizerDeletePXCSSL         = "delete-ssl"
)

type PXCProvider struct {
	providers.ProviderOptions
	*pxcv1.PerconaXtraDBCluster
	ctx         context.Context
	clusterType common.ClusterType
}

func New(
	ctx context.Context,
	opts providers.ProviderOptions,
) (*PXCProvider, error) {
	pxc := &pxcv1.PerconaXtraDBCluster{}
	client := opts.C
	err := client.Get(ctx, types.NamespacedName{Name: opts.DB.GetName(), Namespace: opts.DB.GetNamespace()}, pxc)
	if err != nil && !k8serrors.IsNotFound(err) {
		return nil, err
	}

	// Add necessary finalizers.
	finalizers := []string{
		finalizerDeletePXCPodsInOrder,
		finalizerDeletePXCPVC,
		finalizerDeletePXCSSL,
	}
	for _, f := range finalizers {
		controllerutil.AddFinalizer(pxc, f)
	}

	dbEngine, err := common.GetDatabaseEngine(client, common.PXCDeploymentName, opts.DB.GetNamespace())
	if err != nil {
		return nil, err
	}
	opts.DBEngine = dbEngine

	pxc.Spec = defaultSpec()

	p := &PXCProvider{
		PerconaXtraDBCluster: pxc,
		ctx:                  ctx,
		ProviderOptions:      opts,
	}

	if err := p.ensureDefaults(); err != nil {
		return nil, err
	}
	if err := p.handlePXCOperatorVersion(); err != nil {
		return nil, err
	}
	if err := p.handlePXCRestores(); err != nil {
		return nil, err
	}
	if err := p.handleClusterTypeConfig(); err != nil {
		return nil, err
	}
	if err := common.ApplyTemplate(ctx, opts.C, opts.DB, pxc); err != nil {
		return nil, err
	}
	return p, nil
}

func (p *PXCProvider) Apply() everestv1alpha1.Applier {
	return &pxcApplier{
		PXCProvider: p,
	}
}

// handleClusterTypeConfig cluster type specific configuration (if any).
func (p *PXCProvider) handleClusterTypeConfig() error {
	ct, err := common.GetClusterType(p.ctx, p.C)
	if err != nil {
		return err

	}
	p.clusterType = ct
	if ct == common.ClusterTypeEKS {
		affinity := &pxcv1.PodAffinity{
			TopologyKey: pointer.ToString(common.TopologyKeyHostname),
		}
		p.PerconaXtraDBCluster.Spec.PXC.PodSpec.Affinity = affinity
		p.PerconaXtraDBCluster.Spec.HAProxy.PodSpec.Affinity = affinity
		p.PerconaXtraDBCluster.Spec.ProxySQL.Affinity = affinity
	}
	return nil
}

// handlePXCOperatorVersion updates the .spec.CRVersion (if needed).
func (p *PXCProvider) handlePXCOperatorVersion() error {
	pxc := p.PerconaXtraDBCluster
	v, err := common.GetOperatorVersion(p.ctx, p.C, types.NamespacedName{
		Name:      common.PXCDeploymentName,
		Namespace: p.DB.GetNamespace(),
	})
	if err != nil {
		return err
	}
	pxc.TypeMeta = metav1.TypeMeta{
		APIVersion: v.ToAPIVersion(common.PXCAPIGroup),
		Kind:       common.PerconaXtraDBClusterKind,
	}
	crVersion := v.ToCRVersion()
	if pxc.Spec.CRVersion != "" {
		crVersion = pxc.Spec.CRVersion
	}
	pxc.Spec.CRVersion = crVersion
	return nil
}

// handlePXCRestores is a helper for watching PXC restores and reconciling the db.Paused.
// During the restoration of PXC clusters
// They need to be shutted down
//
// It's not a good idea to shutdown them from DatabaseCluster object perspective
// hence we have this piece of the migration of spec.pause field
// from PerconaXtraDBCluster object to a DatabaseCluster object.
func (p *PXCProvider) handlePXCRestores() error {
	db := p.DB
	pxc := p.PerconaXtraDBCluster
	if pxc.Spec.Pause != db.Spec.Paused {
		restores := &everestv1alpha1.DatabaseClusterRestoreList{}
		listOps := &client.ListOptions{
			FieldSelector: fields.OneTermEqualSelector(common.DBClusterRestoreDBClusterNameField, db.GetName()),
			Namespace:     db.GetNamespace(),
		}
		err := p.C.List(p.ctx, restores, listOps)
		if err != nil {
			return err
		}
		for _, restore := range restores.Items {
			if !restore.IsComplete(db.Spec.Engine.Type) {
				db.Spec.Paused = pxc.Spec.Pause
				return p.C.Update(p.ctx, db)
			}
		}
	}
	return nil
}

func (p *PXCProvider) ensureDefaults() error {
	db := p.DB
	updated := false
	if db.Spec.Proxy.Type == "" {
		db.Spec.Proxy.Type = everestv1alpha1.ProxyTypeHAProxy
		updated = true
	}

	if db.Spec.Engine.Config == "" {
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
		return p.C.Update(p.ctx, db)
	}
	return nil
}

func (p *PXCProvider) Status() (everestv1alpha1.DatabaseClusterStatus, error) {
	status := p.DB.Status
	pxc := p.PerconaXtraDBCluster

	status.Status = everestv1alpha1.AppState(p.PerconaXtraDBCluster.Status.Status)
	status.Hostname = pxc.Status.Host
	status.Ready = pxc.Status.Ready
	status.Size = pxc.Status.Size
	status.Message = strings.Join(pxc.Status.Messages, ";")
	status.Port = 3306

	// If a restore is running for this database, set the database status to restoring.
	if restoring, err := common.IsDatabaseClusterRestoreRunning(p.ctx, p.C, p.DB.Spec.Engine.Type, p.DB.GetName(), p.DB.GetNamespace()); err != nil {
		return status, err
	} else if restoring {
		status.Status = everestv1alpha1.AppStateRestoring
	}
	return status, nil
}

func (p *PXCProvider) Cleanup(db *everestv1alpha1.DatabaseCluster) (bool, error) {
	// Nothing to do
	return true, nil
}

func (p *PXCProvider) DBObject() client.Object {
	return p.PerconaXtraDBCluster
}
