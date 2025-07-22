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

package pxc

import (
	"context"
	"errors"
	"fmt"
	"testing"

	pxcv1 "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	everestv1alpha1 "github.com/percona/everest-operator/api/v1alpha1"
	"github.com/percona/everest-operator/internal/consts"
	"github.com/percona/everest-operator/internal/controller/common"
	"github.com/percona/everest-operator/internal/controller/providers"
)

func TestAddBackupStorages(t *testing.T) {
	t.Parallel()
	ns := "some_namespace"
	dbName := "test"
	uid := "some_uid"
	testPXCSpec := &pxcv1.BackupStorageSpec{
		Type: pxcv1.BackupStorageS3,
		S3: &pxcv1.BackupStorageS3Spec{
			Bucket: fmt.Sprintf("/%s/%s", dbName, uid),
		},
		Resources: corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("1G"),
				corev1.ResourceCPU:    resource.MustParse("600m"),
			},
		},
	}

	backup1 := everestv1alpha1.DatabaseClusterBackup{ObjectMeta: metav1.ObjectMeta{Name: "backup1", Namespace: ns}, Spec: everestv1alpha1.DatabaseClusterBackupSpec{
		BackupStorageName: "storage-1",
	}}
	backup2 := everestv1alpha1.DatabaseClusterBackup{ObjectMeta: metav1.ObjectMeta{Name: "backup2", Namespace: ns}, Spec: everestv1alpha1.DatabaseClusterBackupSpec{
		BackupStorageName: "storage-2",
	}}
	storage1 := everestv1alpha1.BackupStorage{ObjectMeta: metav1.ObjectMeta{Name: "storage-1", Namespace: ns}, Spec: everestv1alpha1.BackupStorageSpec{
		Type: everestv1alpha1.BackupStorageTypeS3,
	}}
	storage2 := everestv1alpha1.BackupStorage{ObjectMeta: metav1.ObjectMeta{Name: "storage-2", Namespace: ns}, Spec: everestv1alpha1.BackupStorageSpec{
		Type: everestv1alpha1.BackupStorageTypeS3,
	}}

	appl := mockApplier(t,
		metav1.ObjectMeta{
			Name:      dbName,
			UID:       types.UID(uid),
			Namespace: ns,
		},
		&backup1, &backup2, &storage1, &storage2)

	type tcase struct {
		name       string
		backups    []everestv1alpha1.DatabaseClusterBackup
		dataSource *everestv1alpha1.DataSource
		expected   map[string]*pxcv1.BackupStorageSpec
		backup     everestv1alpha1.DatabaseClusterBackup
		error      error
	}
	tcases := []tcase{
		{
			name:       "no backups no datasource",
			backups:    []everestv1alpha1.DatabaseClusterBackup{},
			dataSource: nil,
			expected:   map[string]*pxcv1.BackupStorageSpec{},
			error:      nil,
		},
		{
			name:    "no backups dataSource with backupSource",
			backups: []everestv1alpha1.DatabaseClusterBackup{},
			dataSource: &everestv1alpha1.DataSource{BackupSource: &everestv1alpha1.BackupSource{
				Path:              "some/path",
				BackupStorageName: "storage-1",
			}},
			expected: map[string]*pxcv1.BackupStorageSpec{
				"storage-1": testPXCSpec,
			},
			error: nil,
		},
		{
			name:    "no backups dataSource with backupName",
			backups: []everestv1alpha1.DatabaseClusterBackup{},
			dataSource: &everestv1alpha1.DataSource{
				DBClusterBackupName: "backup1",
			},
			expected: map[string]*pxcv1.BackupStorageSpec{
				"storage-1": testPXCSpec,
			},
			error: nil,
		},
		{
			name:    "err no such backup",
			backups: []everestv1alpha1.DatabaseClusterBackup{},
			dataSource: &everestv1alpha1.DataSource{
				DBClusterBackupName: "backup3",
			},
			expected: nil,
			error:    errors.New(`databaseclusterbackups.everest.percona.com "backup3" not found`),
		},
		{
			name:       "none of the backup name and backupSource defined",
			backups:    []everestv1alpha1.DatabaseClusterBackup{},
			dataSource: &everestv1alpha1.DataSource{},
			expected:   nil,
			error:      errInvalidDataSourceConfiguration,
		},
		{
			name:       "one backup no backupSource",
			backups:    []everestv1alpha1.DatabaseClusterBackup{backup1},
			dataSource: nil,
			expected: map[string]*pxcv1.BackupStorageSpec{
				"storage-1": testPXCSpec,
			},
			error: nil,
		},
		{
			name:       "two backups different storages no backupSource",
			backups:    []everestv1alpha1.DatabaseClusterBackup{backup1, backup2},
			dataSource: nil,
			expected: map[string]*pxcv1.BackupStorageSpec{
				"storage-1": testPXCSpec,
				"storage-2": testPXCSpec,
			},
			error: nil,
		},
	}
	for _, tc := range tcases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			storages := map[string]*pxcv1.BackupStorageSpec{}
			err := appl.addBackupStorages(tc.backups, tc.dataSource, storages)
			if tc.error != nil {
				require.Error(t, err)
				assert.Equal(t, tc.error.Error(), err.Error())
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.expected, storages)
		})
	}
}

func mockApplier(t *testing.T, dbMeta metav1.ObjectMeta, objs ...client.Object) applier {
	t.Helper()
	ctx := t.Context()
	scheme := runtime.NewScheme()
	utilruntime.Must(everestv1alpha1.AddToScheme(scheme))
	utilruntime.Must(pxcv1.SchemeBuilder.AddToScheme(scheme))
	utilruntime.Must(appsv1.AddToScheme(scheme))
	utilruntime.Must(storagev1.AddToScheme(scheme))

	mockDatabase := &everestv1alpha1.DatabaseCluster{
		ObjectMeta: dbMeta,
		Spec: everestv1alpha1.DatabaseClusterSpec{
			Engine: everestv1alpha1.Engine{Version: "8.0.0"},
			Proxy:  everestv1alpha1.Proxy{Type: "haproxy"},
		},
	}

	engine := everestv1alpha1.DatabaseEngine{ObjectMeta: metav1.ObjectMeta{Name: consts.PXCDeploymentName, Namespace: mockDatabase.GetNamespace()}, Spec: everestv1alpha1.DatabaseEngineSpec{}}
	deployment := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "percona-xtradb-cluster-operator",
			Namespace: "some_namespace",
		},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Image: "percona/percona-xtradb-cluster-operator:1.12.0",
						},
					},
				},
			},
		},
	}
	storageClassAWS := storagev1.StorageClass{
		ObjectMeta:  metav1.ObjectMeta{Name: "aws-storage"},
		Provisioner: "kubernetes.io/aws-ebs",
	}

	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(&engine, &deployment, &storageClassAWS).WithObjects(objs...).Build()
	unstrCl := unstructuredClient{cl}

	dbEngine, err := common.GetDatabaseEngine(ctx, cl, consts.PXCDeploymentName, mockDatabase.GetNamespace())
	require.NoError(t, err)
	p, err := New(ctx, providers.ProviderOptions{
		C:        unstrCl,
		DB:       mockDatabase,
		DBEngine: dbEngine,
	})
	require.NoError(t, err)
	return applier{
		Provider: p,
		ctx:      ctx,
	}
}

// fake client that can work with StorageClassList as UnstructuredList.
type unstructuredClient struct {
	client.Client
}

func (c unstructuredClient) List(ctx context.Context, obj client.ObjectList, opts ...client.ListOption) error {
	storageList := &storagev1.StorageClassList{}
	err := c.Client.List(ctx, storageList, opts...)
	if err != nil {
		return err
	}
	// Adapt the StorageClassList into an Unstructured object
	unstructuredList := &unstructured.UnstructuredList{} // Create the object
	unstructuredList.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "storage.k8s.io",
		Kind:    "StorageClassList",
		Version: "v1",
	})

	// convert each StorageClass object into an unstructured format.
	items := make([]unstructured.Unstructured, len(storageList.Items))
	for i, item := range storageList.Items {
		unstructuredItem, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&item)
		if err != nil {
			return err
		}
		items[i] = unstructured.Unstructured{Object: unstructuredItem} // Assign the result
	}
	unstructuredList.Items = items
	err = runtime.DefaultUnstructuredConverter.FromUnstructured(unstructuredList.Object, obj)
	if err != nil {
		return err
	}
	return nil
}
