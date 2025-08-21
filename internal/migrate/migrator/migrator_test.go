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

package migrator

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	v1 "k8s.io/api/coordination/v1"
	storagev1 "k8s.io/api/storage/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/percona/everest-operator/api/v1alpha1"
)

const everestSystemNs = "everest-system"

func TestMigrateNonAWS(t *testing.T) {
	m := Migrator{
		l:               zap.New(zap.UseDevMode(true), zap.WriteTo(os.Stdout)),
		currentVersion:  "0.0.0",
		systemNamespace: everestSystemNs,
	}
	m.client = fake.NewClientBuilder().WithScheme(m.BuildScheme()).Build()
	ctx := context.Background()

	err, info := m.Migrate(ctx)
	// check the migration is not performed on cluster types other than EKS
	assert.NoError(t, err)
	assert.Equal(t, "cluster type is not EKS, applying empty migration", info)

	// check the Lease is created and contains the current version
	checkLeaseIsCreatedAndUpdated(t, m, ctx)
}

func TestMigrateAWSNoDBs(t *testing.T) {
	storageClassAWS := storagev1.StorageClass{
		ObjectMeta:  metav1.ObjectMeta{Name: "aws-storage"},
		Provisioner: "kubernetes.io/aws-ebs",
	}
	storageClassList := storagev1.StorageClassList{
		TypeMeta: metav1.TypeMeta{},
		ListMeta: metav1.ListMeta{},
		Items:    []storagev1.StorageClass{storageClassAWS},
	}
	m := Migrator{
		l:               zap.New(zap.UseDevMode(true), zap.WriteTo(os.Stdout)),
		currentVersion:  "0.0.0",
		systemNamespace: "everest-system",
	}
	m.client = fake.NewClientBuilder().WithScheme(m.BuildScheme()).WithLists(&storageClassList).Build()

	ctx := context.Background()

	err, info := m.Migrate(ctx)
	// check the migration is performed successfully
	assert.Nil(t, err)
	assert.Equal(t, "migration is performed successfully", info)

	checkLeaseIsCreatedAndUpdated(t, m, ctx)
}

func TestMigrateAWSAlreadyPerformed(t *testing.T) {
	storageClassAWS := storagev1.StorageClass{
		ObjectMeta:  metav1.ObjectMeta{Name: "aws-storage"},
		Provisioner: "kubernetes.io/aws-ebs",
	}
	storageClassList := storagev1.StorageClassList{
		TypeMeta: metav1.TypeMeta{},
		ListMeta: metav1.ListMeta{},
		Items:    []storagev1.StorageClass{storageClassAWS},
	}
	// already have a lease with the same last migration version
	lease := v1.Lease{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      leaseName,
			Namespace: everestSystemNs,
			Annotations: map[string]string{
				lastMigrationVersionAnnotationName: "0.0.0",
			},
		},
	}
	m := Migrator{
		l:               zap.New(zap.UseDevMode(true), zap.WriteTo(os.Stdout)),
		currentVersion:  "0.0.0",
		systemNamespace: "everest-system",
	}
	m.client = fake.NewClientBuilder().WithScheme(m.BuildScheme()).WithLists(&storageClassList).WithObjects(&lease).Build()
	ctx := context.Background()

	err, info := m.Migrate(ctx)
	// check we detected that migration is already performed
	assert.Nil(t, err)
	assert.Equal(t, "migration is already performed", info)
}

func TestMigrateAWSNonExposedClusters(t *testing.T) {
	storageClassAWS := storagev1.StorageClass{
		ObjectMeta:  metav1.ObjectMeta{Name: "aws-storage"},
		Provisioner: "kubernetes.io/aws-ebs",
	}
	storageClassList := storagev1.StorageClassList{
		TypeMeta: metav1.TypeMeta{},
		ListMeta: metav1.ListMeta{},
		Items:    []storagev1.StorageClass{storageClassAWS},
	}
	ns := "ns1"
	db1 := v1alpha1.DatabaseCluster{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "db1",
			Namespace: ns,
		},
		Spec: v1alpha1.DatabaseClusterSpec{
			Proxy: v1alpha1.Proxy{
				Expose: v1alpha1.Expose{
					Type: "internal",
				},
			},
		},
		Status: v1alpha1.DatabaseClusterStatus{},
	}

	m := Migrator{
		l:               zap.New(zap.UseDevMode(true), zap.WriteTo(os.Stdout)),
		currentVersion:  "0.0.0",
		systemNamespace: "everest-system",
	}
	m.client = fake.NewClientBuilder().WithScheme(m.BuildScheme()).WithLists(&storageClassList).WithObjects(&db1).Build()

	ctx := context.Background()

	err, info := m.Migrate(ctx)
	assert.Nil(t, err)
	assert.Equal(t, "migration is performed successfully", info)

	// check the Lease is created and updated with the current version
	checkLeaseIsCreatedAndUpdated(t, m, ctx)

	// check that the default LoadBalancerConfigName is not assigned
	updatedDB := &v1alpha1.DatabaseCluster{}
	err = m.client.Get(ctx, types.NamespacedName{
		Name:      "db1",
		Namespace: ns,
	}, updatedDB, nil)
	assert.NoError(t, err)
	assert.Equal(t, db1.Name, updatedDB.Name)
	assert.Equal(t, "", updatedDB.Spec.Proxy.Expose.LoadBalancerConfigName)
}

func TestMigrateAWSExposedClusters(t *testing.T) {
	storageClassAWS := storagev1.StorageClass{
		ObjectMeta:  metav1.ObjectMeta{Name: "aws-storage"},
		Provisioner: "kubernetes.io/aws-ebs",
	}
	storageClassList := storagev1.StorageClassList{
		TypeMeta: metav1.TypeMeta{},
		ListMeta: metav1.ListMeta{},
		Items:    []storagev1.StorageClass{storageClassAWS},
	}
	ns := "ns1"
	db1 := v1alpha1.DatabaseCluster{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "db1",
			Namespace: ns,
		},
		Spec: v1alpha1.DatabaseClusterSpec{
			Proxy: v1alpha1.Proxy{
				Expose: v1alpha1.Expose{
					Type: "external",
				},
			},
		},
		Status: v1alpha1.DatabaseClusterStatus{},
	}

	m := Migrator{
		l:               zap.New(zap.UseDevMode(true), zap.WriteTo(os.Stdout)),
		currentVersion:  "0.0.0",
		systemNamespace: "everest-system",
	}
	m.client = fake.NewClientBuilder().WithScheme(m.BuildScheme()).WithLists(&storageClassList).WithObjects(&db1).Build()

	ctx := context.Background()

	err, info := m.Migrate(ctx)
	assert.Nil(t, err)
	assert.Equal(t, "migration is performed successfully", info)

	// check the Lease is created and updated with the current version
	checkLeaseIsCreatedAndUpdated(t, m, ctx)

	// check that the default LoadBalancerConfigName is assigned
	updatedDB := &v1alpha1.DatabaseCluster{}
	m.client.Get(ctx, types.NamespacedName{
		Name:      "db1",
		Namespace: ns,
	}, updatedDB, nil)
	assert.Equal(t, db1.Name, updatedDB.Name)
	assert.Equal(t, defaultEKSLoadBalancerConfigName, updatedDB.Spec.Proxy.Expose.LoadBalancerConfigName)
}

func checkLeaseIsCreatedAndUpdated(t *testing.T, m Migrator, ctx context.Context) {
	// check the Lease is created and contains the current version
	lease := &v1.Lease{}
	err := m.client.Get(ctx, types.NamespacedName{
		Name:      leaseName,
		Namespace: everestSystemNs,
	}, lease, nil)
	assert.NoError(t, err)
	assert.Equal(t, map[string]string{lastMigrationVersionAnnotationName: "0.0.0"}, lease.Annotations)
}
