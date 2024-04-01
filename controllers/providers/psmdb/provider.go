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

// Package psmdb contains the provider for Percona Server for MongoDB.
package psmdb

import (
	"context"

	"github.com/AlekSi/pointer"
	psmdbv1 "github.com/percona/percona-server-mongodb-operator/pkg/apis/psmdb/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	everestv1alpha1 "github.com/percona/everest-operator/api/v1alpha1"
	"github.com/percona/everest-operator/controllers/common"
	"github.com/percona/everest-operator/controllers/providers"
)

const (
	finalizerDeletePSMDBPodsInOrder = "delete-psmdb-pods-in-order"
	finalizerDeletePSMDBPVC         = "delete-psmdb-pvc"
)

// Provider is a provider for Percona Server for MongoDB.
type Provider struct {
	*psmdbv1.PerconaServerMongoDB
	providers.ProviderOptions
}

// New returns a new provider for Percona Server for MongoDB.
func New(
	ctx context.Context,
	opts providers.ProviderOptions,
) (*Provider, error) {
	client := opts.C

	psmdb := &psmdbv1.PerconaServerMongoDB{}
	err := client.Get(ctx,
		types.NamespacedName{
			Name:      opts.DB.GetName(),
			Namespace: opts.DB.GetNamespace(),
		},
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

	dbEngine, err := common.GetDatabaseEngine(ctx, client, common.PSMDBDeploymentName, opts.DB.GetNamespace())
	if err != nil {
		return nil, err
	}
	opts.DBEngine = dbEngine

	psmdb.Spec = defaultSpec()
	p := &Provider{
		PerconaServerMongoDB: psmdb,
		ProviderOptions:      opts,
	}
	if err := p.handleOperatorVersion(ctx); err != nil {
		return nil, err
	}
	if err := p.handleClusterTypeConfig(ctx); err != nil {
		return nil, err
	}
	return p, nil
}

func (p *Provider) handleOperatorVersion(ctx context.Context) error {
	psmdb := p.PerconaServerMongoDB
	v, err := common.GetOperatorVersion(ctx, p.C, types.NamespacedName{
		Name:      common.PSMDBDeploymentName,
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
func (p *Provider) handleClusterTypeConfig(ctx context.Context) error {
	psmdb := p.PerconaServerMongoDB
	ct, err := common.GetClusterType(ctx, p.C)
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

// Apply returns the applier for Percona Server for MongoDB.
//
//nolint:ireturn
func (p *Provider) Apply(ctx context.Context) everestv1alpha1.Applier {
	return &applier{
		Provider: p,
		ctx:      ctx,
	}
}

// Status builds the DatabaseCluster Status based on the current state of the PerconaServerMongoDB.
func (p *Provider) Status(ctx context.Context) (everestv1alpha1.DatabaseClusterStatus, error) {
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
	if restoring, err := common.IsDatabaseClusterRestoreRunning(ctx, p.C, p.DB.Spec.Engine.Type, p.DB.GetName(), p.DB.GetNamespace()); err != nil {
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

// DBObject returns the PerconaServerMongoDB object.
//
//nolint:ireturn
func (p *Provider) DBObject() client.Object {
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
