package psmdb

import (
	"context"

	"github.com/AlekSi/pointer"
	everestv1alpha1 "github.com/percona/everest-operator/api/v1alpha1"
	"github.com/percona/everest-operator/controllers/common"
	"github.com/percona/everest-operator/controllers/providers"
	psmdbv1 "github.com/percona/percona-server-mongodb-operator/pkg/apis/psmdb/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	finalizerDeletePSMDBPodsInOrder = "delete-psmdb-pods-in-order"
	finalizerDeletePSMDBPVC         = "delete-psmdb-pvc"
)

type PSMDBProvider struct {
	*psmdbv1.PerconaServerMongoDB
	providers.ProviderOptions
	ctx context.Context
}

func New(
	ctx context.Context,
	opts providers.ProviderOptions,
) (*PSMDBProvider, error) {
	client := opts.C

	psmdb := &psmdbv1.PerconaServerMongoDB{}
	err := client.Get(ctx,
		types.NamespacedName{
			Name:      opts.DB.GetName(),
			Namespace: opts.DB.GetNamespace()},
		psmdb)
	if err != nil && !k8serrors.IsNotFound(err) {
		return nil, err
	}

	// Add necessary finalizers.
	finalizers := []string{
		finalizerDeletePSMDBPodsInOrder,
		finalizerDeletePSMDBPVC,
	}
	for _, f := range finalizers {
		controllerutil.AddFinalizer(psmdb, f)
	}

	dbEngine, err := common.GetDatabaseEngine(client, common.PGDeploymentName, opts.DB.GetNamespace())
	if err != nil {
		return nil, err
	}
	opts.DBEngine = dbEngine

	psmdb.Spec = defaultSpec()
	p := &PSMDBProvider{
		PerconaServerMongoDB: psmdb,
		ProviderOptions:      opts,
		ctx:                  ctx,
	}
	if err := p.handleOperatorVersion(); err != nil {
		return nil, err
	}
	if err := p.handleClusterTypeConfig(); err != nil {
		return nil, err
	}
	if err := common.ApplyTemplate(ctx, opts.C, opts.DB, psmdb); err != nil {
		return nil, err
	}
	return p, nil
}

func (p *PSMDBProvider) handleOperatorVersion() error {
	psmdb := p.PerconaServerMongoDB
	v, err := common.GetOperatorVersion(p.ctx, p.C, types.NamespacedName{
		Name:      common.PGDeploymentName,
		Namespace: p.DB.GetNamespace(),
	})
	if err != nil {
		return err
	}
	psmdb.TypeMeta = metav1.TypeMeta{
		APIVersion: v.ToAPIVersion(common.PSMDBAPIGroup),
		Kind:       common.PerconaServerMongoDBKind,
	}
	crVersion := v.ToCRVersion()
	if psmdb.Spec.CRVersion != "" {
		crVersion = psmdb.Spec.CRVersion
	}
	psmdb.Spec.CRVersion = crVersion
	return nil
}

// handleClusterTypeConfig cluster type specific configuration (if any).
func (p *PSMDBProvider) handleClusterTypeConfig() error {
	psmdb := p.PerconaServerMongoDB
	ct, err := common.GetClusterType(p.ctx, p.C)
	if err != nil {
		return err

	}
	if ct == common.ClusterTypeEKS {
		affinity := &psmdbv1.PodAffinity{
			TopologyKey: pointer.ToString("kubernetes.io/hostname"),
		}
		psmdb.Spec.Replsets[0].MultiAZ.Affinity = affinity
	}
	return nil
}

func (p *PSMDBProvider) Apply() everestv1alpha1.Applier {
	return &psmdbApplier{p}
}

func (p *PSMDBProvider) Status() (everestv1alpha1.DatabaseClusterStatus, error) {
	status := p.DB.Status
	psmdb := p.PerconaServerMongoDB

	activeStorage := getActiveStorage(psmdb)
	status.Status = everestv1alpha1.AppState(psmdb.Status.State)
	status.Hostname = psmdb.Status.Host
	status.Ready = psmdb.Status.Ready
	status.Size = psmdb.Status.Size
	message := psmdb.Status.Message
	conditions := psmdb.Status.Conditions
	if message == "" && len(conditions) != 0 {
		message = conditions[len(conditions)-1].Message
	}
	status.Message = message
	status.Port = 27017
	status.ActiveStorage = activeStorage
	// If a restore is running for this database, set the database status to restoring.
	if restoring, err := common.IsDatabaseClusterRestoreRunning(p.ctx, p.C, p.DB.Spec.Engine.Type, p.DB.GetName(), p.DB.GetNamespace()); err != nil {
		return status, err
	} else if restoring {
		status.Status = everestv1alpha1.AppStateRestoring
	}
	return status, nil
}

func (p *PSMDBProvider) Cleanup(db *everestv1alpha1.DatabaseCluster) (bool, error) {
	// Nothing to do
	return true, nil
}

func (p *PSMDBProvider) DBObject() client.Object {
	return p.PerconaServerMongoDB
}

func defaultSpec() psmdbv1.PerconaServerMongoDBSpec {
	maxUnavailable := intstr.FromInt(1)
	return psmdbv1.PerconaServerMongoDBSpec{
		UpdateStrategy: psmdbv1.SmartUpdateStatefulSetStrategyType,
		UpgradeOptions: psmdbv1.UpgradeOptions{
			Apply:    "disabled",
			Schedule: "0 4 * * *",
		},
		PMM: psmdbv1.PMMSpec{
			Enabled: false,
			Resources: corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("300M"),
					corev1.ResourceCPU:    resource.MustParse("500m"),
				},
			},
		},
		Replsets: []*psmdbv1.ReplsetSpec{
			{
				Name: "rs0",
				MultiAZ: psmdbv1.MultiAZ{
					PodDisruptionBudget: &psmdbv1.PodDisruptionBudgetSpec{
						MaxUnavailable: &maxUnavailable,
					},
					Affinity: &psmdbv1.PodAffinity{
						TopologyKey: pointer.ToString(psmdbv1.AffinityOff),
					},
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{},
					},
				},
			},
		},
		Sharding: psmdbv1.Sharding{
			Enabled: false,
		},
	}
}

func getActiveStorage(psmdb *psmdbv1.PerconaServerMongoDB) string {
	for name := range psmdb.Spec.Backup.Storages {
		return name
	}
	return ""
}
