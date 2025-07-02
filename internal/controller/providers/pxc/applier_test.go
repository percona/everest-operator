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
	"testing"

	pxcv1 "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	everestv1alpha1 "github.com/percona/everest-operator/api/v1alpha1"
)

func TestAddBackupStorages(t *testing.T) {
	t.Parallel()
	testPXCSpec := &pxcv1.BackupStorageSpec{
		Type: pxcv1.BackupStorageS3,
		S3:   &pxcv1.BackupStorageS3Spec{},
	}
	mockGetStoragesSpec := func(string, string, *everestv1alpha1.DatabaseCluster) (*pxcv1.BackupStorageSpec, error) { //nolint:unparam
		return testPXCSpec, nil
	}
	mockDatabase := &everestv1alpha1.DatabaseCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test",
			UID:  "some_uid",
		},
	}

	scheme := runtime.NewScheme()
	utilruntime.Must(everestv1alpha1.AddToScheme(scheme))

	backup1 := everestv1alpha1.DatabaseClusterBackup{ObjectMeta: metav1.ObjectMeta{Name: "backup1"}, Spec: everestv1alpha1.DatabaseClusterBackupSpec{
		BackupStorageName: "storage-1",
	}}
	backup2 := everestv1alpha1.DatabaseClusterBackup{ObjectMeta: metav1.ObjectMeta{Name: "backup2"}, Spec: everestv1alpha1.DatabaseClusterBackupSpec{
		BackupStorageName: "storage-2",
	}}
	cl := fake.NewClientBuilder().WithScheme(scheme).WithObjects(&backup1, &backup2).Build()

	type tcase struct {
		name            string
		backups         []everestv1alpha1.DatabaseClusterBackup
		dataSource      *everestv1alpha1.DataSource
		expected        map[string]*pxcv1.BackupStorageSpec
		database        *everestv1alpha1.DatabaseCluster
		getStoragesSpec func(string, string, *everestv1alpha1.DatabaseCluster) (*pxcv1.BackupStorageSpec, error)
		cGet            func(context.Context, client.ObjectKey, client.Object, ...client.GetOption) error
		backup          everestv1alpha1.DatabaseClusterBackup
		error           error
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
			err := addBackupStorages(tc.backups, tc.dataSource, storages, mockDatabase, mockGetStoragesSpec, cl.Get)
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
