package pg

import (
	"context"

	everestv1alpha1 "github.com/percona/everest-operator/api/v1alpha1"
	"github.com/percona/everest-operator/controllers/common"
	"github.com/percona/everest-operator/controllers/providers"
	pgv2 "github.com/percona/percona-postgresql-operator/pkg/apis/pgv2.percona.com/v2"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	pgAPIGroup = "pgv2.percona.com"

	finalizerDeletePGPVC = "percona.com/delete-pvc"
	finalizerDeletePGSSL = "percona.com/delete-ssl"
)

type PGProvider struct {
	*pgv2.PerconaPGCluster
	providers.ProviderOptions
	ctx context.Context
}

func New(
	ctx context.Context,
	opts providers.ProviderOptions,
) (*PGProvider, error) {
	client := opts.C
	pg := &pgv2.PerconaPGCluster{}
	err := client.Get(ctx, types.NamespacedName{Name: opts.DB.GetName(), Namespace: opts.DB.GetNamespace()}, pg)
	if err != nil && !k8serrors.IsNotFound(err) {
		return nil, err
	}

	dbEngine, err := common.GetDatabaseEngine(client, common.PGDeploymentName, opts.DB.GetNamespace())
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

	p := &PGProvider{
		PerconaPGCluster: pg,
		ProviderOptions:  opts,
		ctx:              ctx,
	}

	if err := p.handleClusterTypeConfig(); err != nil {
		return nil, err
	}
	if err := p.ensureDataSourceRemoved(opts.DB); err != nil {
		return nil, err
	}
	if err := common.ApplyTemplate(ctx, opts.C, opts.DB, pg); err != nil {
		return nil, err
	}
	return p, nil
}

// DataSource is not needed anymore when db is ready.
// Deleting it to prevent the future restoration conflicts.
func (p *PGProvider) ensureDataSourceRemoved(db *everestv1alpha1.DatabaseCluster) error {
	if db.Status.Status == everestv1alpha1.AppStateReady &&
		db.Spec.DataSource != nil {
		db.Spec.DataSource = nil
		return p.C.Update(p.ctx, db)
	}
	return nil
}

func (p *PGProvider) handlePGOperatorVersion() error {
	pg := p.PerconaPGCluster
	v, err := common.GetOperatorVersion(p.ctx, p.C, types.NamespacedName{
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
		return p.C.Update(p.ctx, pg)
	}
	pg.Spec.CRVersion = crVersion
	return nil
}

// handleClusterTypeConfig cluster type specific configuration (if any).
func (p *PGProvider) handleClusterTypeConfig() error {
	ct, err := common.GetClusterType(p.ctx, p.C)
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
func (p *PGProvider) Apply() everestv1alpha1.Applier {
	return &pgApplier{p}
}

// Status returns the DatabaseClusterStatus based on the PG status.
func (p *PGProvider) Status() (everestv1alpha1.DatabaseClusterStatus, error) {
	ctx := p.ctx
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
func (p *PGProvider) Cleanup(db *everestv1alpha1.DatabaseCluster) (bool, error) {
	// Nothing to do
	return true, nil
}

// DBObject returns the PG object.
func (p *PGProvider) DBObject() client.Object {
	return p.PerconaPGCluster
}
