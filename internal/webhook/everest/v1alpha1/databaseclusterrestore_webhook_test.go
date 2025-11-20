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

package v1alpha1

import (
	"context"
	"fmt"
	"testing"

	everestv1alpha1 "github.com/percona/everest-operator/api/everest/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
)

const (
	dbcrName      = "my-dbcr"
	dbcrNamespace = "default"
	dbName        = "my-database-cluster"
	backupName    = "my-backup"
	backupStorageName = "my-backup-storage"
)

func TestDatabaseClusterRestoreCustomValidator_ValidateCreate(t *testing.T) { //nolint:maintidx
	t.Parallel()

	type testCase struct {
		name         string
		objs         []ctrlclient.Object
		dbcrToCreate *everestv1alpha1.DatabaseClusterRestore
		wantErr      error
	}

	testCases := []testCase{
		// .spec.dbClusterName is empty
		{
			name: ".spec.dbClusterName is empty",
			dbcrToCreate: &everestv1alpha1.DatabaseClusterRestore{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dbcrName,
					Namespace: dbcrNamespace,
				},
				Spec: everestv1alpha1.DatabaseClusterRestoreSpec{},
			},
			wantErr: apierrors.NewInvalid(groupKind, dbcrName, field.ErrorList{
				errRequiredField(dbClusterNamePath),
				field.Invalid(dataSourcePath, "",
					fmt.Sprintf("%s or %s must be specified", dbClusterBackupNamePath, backupSourcePath)),
			}),
		},
		// .spec.dbClusterName is absent
		{
			name: ".spec.dbClusterName is absent",
			dbcrToCreate: &everestv1alpha1.DatabaseClusterRestore{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dbcrName,
					Namespace: dbcrNamespace,
				},
				Spec: everestv1alpha1.DatabaseClusterRestoreSpec{
					DBClusterName: dbName,
				},
			},
			wantErr: apierrors.NewInvalid(groupKind, dbcrName, field.ErrorList{
				field.NotFound(dbClusterNamePath,
					fmt.Sprintf("DatabaseCluster %s not found in namespace %s", dbName, dbcrNamespace)),
				field.Invalid(dataSourcePath, "",
					fmt.Sprintf("%s or %s must be specified", dbClusterBackupNamePath, backupSourcePath)),
			}),
		},
		// .spec.dataSource.dbClusterBackupName and .spec.dataSource.backupSource are both specified
		{
			name: ".spec.dataSource.dbClusterBackupName and .spec.dataSource.backupSource are both specified",
			objs: []ctrlclient.Object{
				&everestv1alpha1.DatabaseCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:      dbName,
						Namespace: dbcrNamespace,
					},
				},
				&everestv1alpha1.BackupStorage{
					ObjectMeta: metav1.ObjectMeta{
						Name:      backupStorageName,
						Namespace: dbcrNamespace,
					},
				},
				&everestv1alpha1.DatabaseClusterBackup{
					ObjectMeta: metav1.ObjectMeta{
						Name:      backupName,
						Namespace: dbcrNamespace,
					},
				},
			},
			dbcrToCreate: &everestv1alpha1.DatabaseClusterRestore{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dbcrName,
					Namespace: dbcrNamespace,
				},
				Spec: everestv1alpha1.DatabaseClusterRestoreSpec{
					DBClusterName: dbName,
					DataSource: everestv1alpha1.DatabaseClusterRestoreDataSource{
						DBClusterBackupName: backupName,
						BackupSource: &everestv1alpha1.BackupSource{
							Path:              "path/to/backup",
							BackupStorageName: "my-backup-storage",
						},
					},
				},
			},
			wantErr: apierrors.NewInvalid(groupKind, dbcrName, field.ErrorList{
				field.Invalid(dataSourcePath, "",
					fmt.Sprintf("either %s or %s must be specified, but not both", dbClusterBackupNamePath, backupSourcePath)),
			}),
		},
		// .spec.dataSource.pitr without date
		{
			name: ".spec.dataSource.pitr without date",
			objs: []ctrlclient.Object{
				&everestv1alpha1.DatabaseCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:      dbName,
						Namespace: dbcrNamespace,
					},
				},
				&everestv1alpha1.BackupStorage{
					ObjectMeta: metav1.ObjectMeta{
						Name:      backupStorageName,
						Namespace: dbcrNamespace,
					},
				},
			},
			dbcrToCreate: &everestv1alpha1.DatabaseClusterRestore{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dbcrName,
					Namespace: dbcrNamespace,
				},
				Spec: everestv1alpha1.DatabaseClusterRestoreSpec{
					DBClusterName: dbName,
					DataSource: everestv1alpha1.DatabaseClusterRestoreDataSource{
						BackupSource: &everestv1alpha1.BackupSource{
							Path:              "path/to/backup",
							BackupStorageName: "my-backup-storage",
						},
						PITR: &everestv1alpha1.PITR{
							Type: everestv1alpha1.PITRTypeDate,
						},
					},
				},
			},
			wantErr: apierrors.NewInvalid(groupKind, dbcrName, field.ErrorList{
				errRequiredField(dataSourcePitrDatePath),
			}),
		},
		// .spec.dataSource.pitr with wrong type
		{
			name: ".spec.dataSource.pitr with wrong type",
			objs: []ctrlclient.Object{
				&everestv1alpha1.DatabaseCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:      dbName,
						Namespace: dbcrNamespace,
					},
				},
				&everestv1alpha1.BackupStorage{
					ObjectMeta: metav1.ObjectMeta{
						Name:      backupStorageName,
						Namespace: dbcrNamespace,
					},
				},
			},
			dbcrToCreate: &everestv1alpha1.DatabaseClusterRestore{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dbcrName,
					Namespace: dbcrNamespace,
				},
				Spec: everestv1alpha1.DatabaseClusterRestoreSpec{
					DBClusterName: dbName,
					DataSource: everestv1alpha1.DatabaseClusterRestoreDataSource{
						BackupSource: &everestv1alpha1.BackupSource{
							Path:              "path/to/backup",
							BackupStorageName: "my-backup-storage",
						},
						PITR: &everestv1alpha1.PITR{
							Type: everestv1alpha1.PITRTypeLatest,
						},
					},
				},
			},
			wantErr: apierrors.NewInvalid(groupKind, dbcrName, field.ErrorList{
				field.NotSupported(dataSourcePitrTypePath, everestv1alpha1.PITRTypeLatest, []everestv1alpha1.PITRType{everestv1alpha1.PITRTypeDate}),
			}),
		},
		// .spec.dataSource.backupSource is empty
		{
			name: ".spec.dataSource.backupSource is empty",
			objs: []ctrlclient.Object{
				&everestv1alpha1.DatabaseCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:      dbName,
						Namespace: dbcrNamespace,
					},
				},
			},
			dbcrToCreate: &everestv1alpha1.DatabaseClusterRestore{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dbcrName,
					Namespace: dbcrNamespace,
				},
				Spec: everestv1alpha1.DatabaseClusterRestoreSpec{
					DBClusterName: dbName,
					DataSource: everestv1alpha1.DatabaseClusterRestoreDataSource{
						BackupSource: &everestv1alpha1.BackupSource{},
					},
				},
			},
			wantErr: apierrors.NewInvalid(groupKind, dbcrName, field.ErrorList{
				errRequiredField(backupSourcePathPath),
				errRequiredField(backupSourceStorageNamePath),
			}),
		},
		// .spec.dataSource.backupSource.backupStorageName is empty
		{
			name: ".spec.dataSource.backupSource.backupStorageName is absent",
			objs: []ctrlclient.Object{
				&everestv1alpha1.DatabaseCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:      dbName,
						Namespace: dbcrNamespace,
					},
				},
			},
			dbcrToCreate: &everestv1alpha1.DatabaseClusterRestore{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dbcrName,
					Namespace: dbcrNamespace,
				},
				Spec: everestv1alpha1.DatabaseClusterRestoreSpec{
					DBClusterName: dbName,
					DataSource: everestv1alpha1.DatabaseClusterRestoreDataSource{
						BackupSource: &everestv1alpha1.BackupSource{
							Path: "path/to/backup",
						},
					},
				},
			},
			wantErr: apierrors.NewInvalid(groupKind, dbcrName, field.ErrorList{
				errRequiredField(backupSourceStorageNamePath),
			}),
		},
		// .spec.dataSource.backupSource.backupStorageName is absent
		{
			name: ".spec.dataSource.backupSource.backupStorageName is absent",
			objs: []ctrlclient.Object{
				&everestv1alpha1.DatabaseCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:      dbName,
						Namespace: dbcrNamespace,
					},
				},
			},
			dbcrToCreate: &everestv1alpha1.DatabaseClusterRestore{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dbcrName,
					Namespace: dbcrNamespace,
				},
				Spec: everestv1alpha1.DatabaseClusterRestoreSpec{
					DBClusterName: dbName,
					DataSource: everestv1alpha1.DatabaseClusterRestoreDataSource{
						BackupSource: &everestv1alpha1.BackupSource{
							Path: "path/to/backup",
							BackupStorageName: backupStorageName,
						},
					},
				},
			},
			wantErr: apierrors.NewInvalid(groupKind, dbcrName, field.ErrorList{
				field.NotFound(backupSourceStorageNamePath,
					fmt.Sprintf("BackupStorage %s not found in namespace %s", backupStorageName, dbcrNamespace)),
			}),
		},
		// .spec.dataSource.backupSource.path is empty
		{
			name: ".spec.dataSource.backupSource.backupStorageName is absent",
			objs: []ctrlclient.Object{
				&everestv1alpha1.DatabaseCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:      dbName,
						Namespace: dbcrNamespace,
					},
				},
				&everestv1alpha1.BackupStorage{
					ObjectMeta: metav1.ObjectMeta{
						Name:      backupStorageName,
						Namespace: dbcrNamespace,
					},
				},
			},
			dbcrToCreate: &everestv1alpha1.DatabaseClusterRestore{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dbcrName,
					Namespace: dbcrNamespace,
				},
				Spec: everestv1alpha1.DatabaseClusterRestoreSpec{
					DBClusterName: dbName,
					DataSource: everestv1alpha1.DatabaseClusterRestoreDataSource{
						BackupSource: &everestv1alpha1.BackupSource{
							BackupStorageName: backupStorageName,
						},
					},
				},
			},
			wantErr: apierrors.NewInvalid(groupKind, dbcrName, field.ErrorList{
				errRequiredField(backupSourcePathPath),
			}),
		},
		// Valid DBCR with backupSource and without pitr
		{
			name: "Valid DBCR with backupSource and without pitr",
			objs: []ctrlclient.Object{
				&everestv1alpha1.DatabaseCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:      dbName,
						Namespace: dbcrNamespace,
					},
				},
				&everestv1alpha1.BackupStorage{
					ObjectMeta: metav1.ObjectMeta{
						Name:      backupStorageName,
						Namespace: dbcrNamespace,
					},
				},
			},
			dbcrToCreate: &everestv1alpha1.DatabaseClusterRestore{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dbcrName,
					Namespace: dbcrNamespace,
				},
				Spec: everestv1alpha1.DatabaseClusterRestoreSpec{
					DBClusterName: dbName,
					DataSource: everestv1alpha1.DatabaseClusterRestoreDataSource{
						BackupSource: &everestv1alpha1.BackupSource{
							Path: "path/to/backup",
							BackupStorageName: backupStorageName,
						},
					},
				},
			},
			wantErr: nil,
		},
		// Valid DBCR with backupSource and with pitr
		{
			name: "Valid DBCR with backupSource and without pitr",
			objs: []ctrlclient.Object{
				&everestv1alpha1.DatabaseCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:      dbName,
						Namespace: dbcrNamespace,
					},
				},
				&everestv1alpha1.BackupStorage{
					ObjectMeta: metav1.ObjectMeta{
						Name:      backupStorageName,
						Namespace: dbcrNamespace,
					},
				},
			},
			dbcrToCreate: &everestv1alpha1.DatabaseClusterRestore{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dbcrName,
					Namespace: dbcrNamespace,
				},
				Spec: everestv1alpha1.DatabaseClusterRestoreSpec{
					DBClusterName: dbName,
					DataSource: everestv1alpha1.DatabaseClusterRestoreDataSource{
						BackupSource: &everestv1alpha1.BackupSource{
							Path: "path/to/backup",
							BackupStorageName: backupStorageName,
						},
						PITR: &everestv1alpha1.PITR{
							Type: everestv1alpha1.PITRTypeDate,
							Date: &everestv1alpha1.RestoreDate{
								Time: metav1.Time{Time: metav1.Now().UTC()},
							},
						},
					},
				},
			},
			wantErr: nil,
		},
		// Valid DBCR with dbClusterBackupName and without pitr
		{
			name: "Valid DBCR with dbClusterBackupName and without pitr",
			objs: []ctrlclient.Object{
				&everestv1alpha1.DatabaseCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:      dbName,
						Namespace: dbcrNamespace,
					},
				},
				&everestv1alpha1.DatabaseClusterBackup{
					ObjectMeta: metav1.ObjectMeta{
						Name:      backupName,
						Namespace: dbcrNamespace,
					},
				},
			},
			dbcrToCreate: &everestv1alpha1.DatabaseClusterRestore{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dbcrName,
					Namespace: dbcrNamespace,
				},
				Spec: everestv1alpha1.DatabaseClusterRestoreSpec{
					DBClusterName: dbName,
					DataSource: everestv1alpha1.DatabaseClusterRestoreDataSource{
						DBClusterBackupName: backupName,
					},
				},
			},
			wantErr: nil,
		},
		// Valid DBCR with dbClusterBackupName and with pitr
		{
			name: "Valid DBCR with dbClusterBackupName and with pitr",
			objs: []ctrlclient.Object{
				&everestv1alpha1.DatabaseCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:      dbName,
						Namespace: dbcrNamespace,
					},
				},
				&everestv1alpha1.DatabaseClusterBackup{
					ObjectMeta: metav1.ObjectMeta{
						Name:      backupName,
						Namespace: dbcrNamespace,
					},
				},
			},
			dbcrToCreate: &everestv1alpha1.DatabaseClusterRestore{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dbcrName,
					Namespace: dbcrNamespace,
				},
				Spec: everestv1alpha1.DatabaseClusterRestoreSpec{
					DBClusterName: dbName,
					DataSource: everestv1alpha1.DatabaseClusterRestoreDataSource{
						DBClusterBackupName: backupName,
						PITR: &everestv1alpha1.PITR{
							Type: everestv1alpha1.PITRTypeDate,
							Date: &everestv1alpha1.RestoreDate{
								Time: metav1.Time{Time: metav1.Now().UTC()},
							},
						},
					},
				},
			},
			wantErr: nil,
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			scheme := runtime.NewScheme()
			utilruntime.Must(clientgoscheme.AddToScheme(scheme))
			utilruntime.Must(everestv1alpha1.AddToScheme(scheme))

			mockClient := fakeclient.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(tc.objs...).
				Build()

			validator := DatabaseClusterRestoreCustomValidator{
				Client: mockClient,
			}

			_, err := validator.ValidateCreate(context.TODO(), tc.dbcrToCreate)
			if tc.wantErr == nil {
				require.NoError(t, err)
				return
			}
			assert.Equal(t, tc.wantErr.Error(), err.Error())
		})
	}
}

func TestDatabaseClusterRestoreCustomValidator_ValidateUpdate(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name    string
		objs    []ctrlclient.Object
		oldDbcr *everestv1alpha1.DatabaseClusterRestore
		newDbcr *everestv1alpha1.DatabaseClusterRestore
		wantErr error
	}

	testCases := []testCase{
		{
			name: "update already marked for deletion DBCR",
			oldDbcr: &everestv1alpha1.DatabaseClusterRestore{
				ObjectMeta: metav1.ObjectMeta{
					Name:              dbcrName,
					Namespace:         dbcrNamespace,
					DeletionTimestamp: &metav1.Time{},
				},
				Spec: everestv1alpha1.DatabaseClusterRestoreSpec{
					DBClusterName: dbName,
				},
			},
			newDbcr: &everestv1alpha1.DatabaseClusterRestore{
				ObjectMeta: metav1.ObjectMeta{
					Name:              dbcrName,
					Namespace:         dbcrNamespace,
				},
				Spec: everestv1alpha1.DatabaseClusterRestoreSpec{
					DBClusterName: dbName,
				},
			},
		},
		{
			name: "update .spec",
			oldDbcr: &everestv1alpha1.DatabaseClusterRestore{
				ObjectMeta: metav1.ObjectMeta{
					Name:              dbcrName,
					Namespace:         dbcrNamespace,
				},
				Spec: everestv1alpha1.DatabaseClusterRestoreSpec{
					DBClusterName: dbName,
				},
			},
			newDbcr: &everestv1alpha1.DatabaseClusterRestore{
				ObjectMeta: metav1.ObjectMeta{
					Name:              dbcrName,
					Namespace:         dbcrNamespace,
				},
				Spec: everestv1alpha1.DatabaseClusterRestoreSpec{
					DBClusterName: dbName,
					DataSource: everestv1alpha1.DatabaseClusterRestoreDataSource{
						DBClusterBackupName: backupName,
					},
				},
			},
			wantErr: apierrors.NewInvalid(groupKind, dbcrName, field.ErrorList{
				errImmutableField(specPath),
			}),
		},
		{
			name: "update .metadata",
			oldDbcr: &everestv1alpha1.DatabaseClusterRestore{
				ObjectMeta: metav1.ObjectMeta{
					Name:              dbcrName,
					Namespace:         dbcrNamespace,
				},
				Spec: everestv1alpha1.DatabaseClusterRestoreSpec{
					DBClusterName: dbName,
				},
			},
			newDbcr: &everestv1alpha1.DatabaseClusterRestore{
				ObjectMeta: metav1.ObjectMeta{
					Name:              dbcrName,
					Namespace:         dbcrNamespace,
					Labels:            map[string]string{"new-label": "new-value"},
				},
				Spec: everestv1alpha1.DatabaseClusterRestoreSpec{
					DBClusterName: dbName,
				},
			},
		},
		{
			name: "update status",
			oldDbcr: &everestv1alpha1.DatabaseClusterRestore{
				ObjectMeta: metav1.ObjectMeta{
					Name:              dbcrName,
					Namespace:         dbcrNamespace,
				},
				Spec: everestv1alpha1.DatabaseClusterRestoreSpec{
					DBClusterName: dbName,
				},
			},
			newDbcr: &everestv1alpha1.DatabaseClusterRestore{
				ObjectMeta: metav1.ObjectMeta{
					Name:              dbcrName,
					Namespace:         dbcrNamespace,
				},
				Spec: everestv1alpha1.DatabaseClusterRestoreSpec{
					DBClusterName: dbName,
				},
				Status: everestv1alpha1.DatabaseClusterRestoreStatus{
					State: everestv1alpha1.RestoreStarting,
				},
			},
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			scheme := runtime.NewScheme()
			utilruntime.Must(clientgoscheme.AddToScheme(scheme))
			utilruntime.Must(everestv1alpha1.AddToScheme(scheme))

			mockClient := fakeclient.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(tc.objs...).
				Build()

			validator := DatabaseClusterRestoreCustomValidator{
				Client: mockClient,
			}

			_, err := validator.ValidateUpdate(context.TODO(), tc.oldDbcr, tc.newDbcr)
			if tc.wantErr == nil {
				require.NoError(t, err)
				return
			}
			assert.Equal(t, tc.wantErr.Error(), err.Error())
		})
	}
}

func TestDatabaseClusterRestoreCustomValidator_ValidateDelete(t *testing.T) {
	t.Parallel()

	type testCase struct {
		name         string
		objs         []ctrlclient.Object
		dbcrToDelete *everestv1alpha1.DatabaseClusterRestore
		wantErr      error
	}

	testCases := []testCase{
		{

		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			scheme := runtime.NewScheme()
			utilruntime.Must(clientgoscheme.AddToScheme(scheme))
			utilruntime.Must(everestv1alpha1.AddToScheme(scheme))

			mockClient := fakeclient.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(tc.objs...).
				Build()

			validator := DatabaseClusterRestoreCustomValidator{
				Client: mockClient,
			}

			_, err := validator.ValidateDelete(context.TODO(), tc.dbcrToDelete)
			if tc.wantErr == nil {
				require.NoError(t, err)
				return
			}
			assert.Equal(t, tc.wantErr.Error(), err.Error())
		})
	}
}
