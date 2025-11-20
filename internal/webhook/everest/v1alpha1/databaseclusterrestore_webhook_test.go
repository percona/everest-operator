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

	everestv1alpha1 "github.com/percona/everest-operator/api/everest/v1alpha1"
)

const (
	dbcrName              = "my-dbcr"
	dbcrNamespace         = "default"
	dbcrDbName            = "my-database-cluster"
	dbcrBackupName        = "my-backup"
	dbcrBackupStorageName = "my-backup-storage"
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
				errRequiredField(dbcrDbClusterNamePath),
				field.Invalid(dbcDataSourcePath, "",
					fmt.Sprintf("%s or %s must be specified", dbcrDbClusterBackupNamePath, dbcrBackupSourcePath)),
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
					DBClusterName: dbcrDbName,
				},
			},
			wantErr: apierrors.NewInvalid(groupKind, dbcrName, field.ErrorList{
				field.NotFound(dbcrDbClusterNamePath,
					fmt.Sprintf("DatabaseCluster %s not found in namespace %s", dbcrDbName, dbcrNamespace)),
				field.Invalid(dbcDataSourcePath, "",
					fmt.Sprintf("%s or %s must be specified", dbcrDbClusterBackupNamePath, dbcrBackupSourcePath)),
			}),
		},
		// .spec.dataSource.dbClusterBackupName and .spec.dataSource.backupSource are both specified
		{
			name: ".spec.dataSource.dbClusterBackupName and .spec.dataSource.backupSource are both specified",
			objs: []ctrlclient.Object{
				&everestv1alpha1.DatabaseCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:      dbcrDbName,
						Namespace: dbcrNamespace,
					},
				},
				&everestv1alpha1.BackupStorage{
					ObjectMeta: metav1.ObjectMeta{
						Name:      dbcrBackupStorageName,
						Namespace: dbcrNamespace,
					},
				},
				&everestv1alpha1.DatabaseClusterBackup{
					ObjectMeta: metav1.ObjectMeta{
						Name:      dbcrBackupName,
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
					DBClusterName: dbcrDbName,
					DataSource: everestv1alpha1.DatabaseClusterRestoreDataSource{
						DBClusterBackupName: dbcrBackupName,
						BackupSource: &everestv1alpha1.BackupSource{
							Path:              "path/to/backup",
							BackupStorageName: "my-backup-storage",
						},
					},
				},
			},
			wantErr: apierrors.NewInvalid(groupKind, dbcrName, field.ErrorList{
				field.Invalid(dbcDataSourcePath, "",
					fmt.Sprintf("either %s or %s must be specified, but not both", dbcrDbClusterBackupNamePath, dbcrBackupSourcePath)),
			}),
		},
		// .spec.dataSource.pitr without date
		{
			name: ".spec.dataSource.pitr without date",
			objs: []ctrlclient.Object{
				&everestv1alpha1.DatabaseCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:      dbcrDbName,
						Namespace: dbcrNamespace,
					},
				},
				&everestv1alpha1.BackupStorage{
					ObjectMeta: metav1.ObjectMeta{
						Name:      dbcrBackupStorageName,
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
					DBClusterName: dbcrDbName,
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
				errRequiredField(dbcrDataSourcePitrDatePath),
			}),
		},
		// .spec.dataSource.pitr with wrong type
		{
			name: ".spec.dataSource.pitr with wrong type",
			objs: []ctrlclient.Object{
				&everestv1alpha1.DatabaseCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:      dbcrDbName,
						Namespace: dbcrNamespace,
					},
				},
				&everestv1alpha1.BackupStorage{
					ObjectMeta: metav1.ObjectMeta{
						Name:      dbcrBackupStorageName,
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
					DBClusterName: dbcrDbName,
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
				field.NotSupported(dbcrDataSourcePitrTypePath, everestv1alpha1.PITRTypeLatest, []everestv1alpha1.PITRType{everestv1alpha1.PITRTypeDate}),
			}),
		},
		// .spec.dataSource.backupSource is empty
		{
			name: ".spec.dataSource.backupSource is empty",
			objs: []ctrlclient.Object{
				&everestv1alpha1.DatabaseCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:      dbcrDbName,
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
					DBClusterName: dbcrDbName,
					DataSource: everestv1alpha1.DatabaseClusterRestoreDataSource{
						BackupSource: &everestv1alpha1.BackupSource{},
					},
				},
			},
			wantErr: apierrors.NewInvalid(groupKind, dbcrName, field.ErrorList{
				errRequiredField(dbcrBackupSourcePathPath),
				errRequiredField(dbcrBackupSourceStorageNamePath),
			}),
		},
		// .spec.dataSource.backupSource.dbcrBackupStorageName is empty
		{
			name: ".spec.dataSource.backupSource.dbcrBackupStorageName is absent",
			objs: []ctrlclient.Object{
				&everestv1alpha1.DatabaseCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:      dbcrDbName,
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
					DBClusterName: dbcrDbName,
					DataSource: everestv1alpha1.DatabaseClusterRestoreDataSource{
						BackupSource: &everestv1alpha1.BackupSource{
							Path: "path/to/backup",
						},
					},
				},
			},
			wantErr: apierrors.NewInvalid(groupKind, dbcrName, field.ErrorList{
				errRequiredField(dbcrBackupSourceStorageNamePath),
			}),
		},
		// .spec.dataSource.backupSource.dbcrBackupStorageName is absent
		{
			name: ".spec.dataSource.backupSource.dbcrBackupStorageName is absent",
			objs: []ctrlclient.Object{
				&everestv1alpha1.DatabaseCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:      dbcrDbName,
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
					DBClusterName: dbcrDbName,
					DataSource: everestv1alpha1.DatabaseClusterRestoreDataSource{
						BackupSource: &everestv1alpha1.BackupSource{
							Path:              "path/to/backup",
							BackupStorageName: dbcrBackupStorageName,
						},
					},
				},
			},
			wantErr: apierrors.NewInvalid(groupKind, dbcrName, field.ErrorList{
				field.NotFound(dbcrBackupSourceStorageNamePath,
					fmt.Sprintf("BackupStorage %s not found in namespace %s", dbcrBackupStorageName, dbcrNamespace)),
			}),
		},
		// .spec.dataSource.backupSource.path is empty
		{
			name: ".spec.dataSource.backupSource.dbcrBackupStorageName is absent",
			objs: []ctrlclient.Object{
				&everestv1alpha1.DatabaseCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:      dbcrDbName,
						Namespace: dbcrNamespace,
					},
				},
				&everestv1alpha1.BackupStorage{
					ObjectMeta: metav1.ObjectMeta{
						Name:      dbcrBackupStorageName,
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
					DBClusterName: dbcrDbName,
					DataSource: everestv1alpha1.DatabaseClusterRestoreDataSource{
						BackupSource: &everestv1alpha1.BackupSource{
							BackupStorageName: dbcrBackupStorageName,
						},
					},
				},
			},
			wantErr: apierrors.NewInvalid(groupKind, dbcrName, field.ErrorList{
				errRequiredField(dbcrBackupSourcePathPath),
			}),
		},
		// Valid DBCR with backupSource and without pitr
		{
			name: "Valid DBCR with backupSource and without pitr",
			objs: []ctrlclient.Object{
				&everestv1alpha1.DatabaseCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:      dbcrDbName,
						Namespace: dbcrNamespace,
					},
				},
				&everestv1alpha1.BackupStorage{
					ObjectMeta: metav1.ObjectMeta{
						Name:      dbcrBackupStorageName,
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
					DBClusterName: dbcrDbName,
					DataSource: everestv1alpha1.DatabaseClusterRestoreDataSource{
						BackupSource: &everestv1alpha1.BackupSource{
							Path:              "path/to/backup",
							BackupStorageName: dbcrBackupStorageName,
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
						Name:      dbcrDbName,
						Namespace: dbcrNamespace,
					},
				},
				&everestv1alpha1.BackupStorage{
					ObjectMeta: metav1.ObjectMeta{
						Name:      dbcrBackupStorageName,
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
					DBClusterName: dbcrDbName,
					DataSource: everestv1alpha1.DatabaseClusterRestoreDataSource{
						BackupSource: &everestv1alpha1.BackupSource{
							Path:              "path/to/backup",
							BackupStorageName: dbcrBackupStorageName,
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
						Name:      dbcrDbName,
						Namespace: dbcrNamespace,
					},
				},
				&everestv1alpha1.DatabaseClusterBackup{
					ObjectMeta: metav1.ObjectMeta{
						Name:      dbcrBackupName,
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
					DBClusterName: dbcrDbName,
					DataSource: everestv1alpha1.DatabaseClusterRestoreDataSource{
						DBClusterBackupName: dbcrBackupName,
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
						Name:      dbcrDbName,
						Namespace: dbcrNamespace,
					},
				},
				&everestv1alpha1.DatabaseClusterBackup{
					ObjectMeta: metav1.ObjectMeta{
						Name:      dbcrBackupName,
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
					DBClusterName: dbcrDbName,
					DataSource: everestv1alpha1.DatabaseClusterRestoreDataSource{
						DBClusterBackupName: dbcrBackupName,
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
					DBClusterName: dbcrDbName,
				},
			},
			newDbcr: &everestv1alpha1.DatabaseClusterRestore{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dbcrName,
					Namespace: dbcrNamespace,
				},
				Spec: everestv1alpha1.DatabaseClusterRestoreSpec{
					DBClusterName: dbcrDbName,
				},
			},
		},
		{
			name: "update .spec",
			oldDbcr: &everestv1alpha1.DatabaseClusterRestore{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dbcrName,
					Namespace: dbcrNamespace,
				},
				Spec: everestv1alpha1.DatabaseClusterRestoreSpec{
					DBClusterName: dbcrDbName,
				},
			},
			newDbcr: &everestv1alpha1.DatabaseClusterRestore{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dbcrName,
					Namespace: dbcrNamespace,
				},
				Spec: everestv1alpha1.DatabaseClusterRestoreSpec{
					DBClusterName: dbcrDbName,
					DataSource: everestv1alpha1.DatabaseClusterRestoreDataSource{
						DBClusterBackupName: dbcrBackupName,
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
					Name:      dbcrName,
					Namespace: dbcrNamespace,
				},
				Spec: everestv1alpha1.DatabaseClusterRestoreSpec{
					DBClusterName: dbcrDbName,
				},
			},
			newDbcr: &everestv1alpha1.DatabaseClusterRestore{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dbcrName,
					Namespace: dbcrNamespace,
					Labels:    map[string]string{"new-label": "new-value"},
				},
				Spec: everestv1alpha1.DatabaseClusterRestoreSpec{
					DBClusterName: dbcrDbName,
				},
			},
		},
		{
			name: "update status",
			oldDbcr: &everestv1alpha1.DatabaseClusterRestore{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dbcrName,
					Namespace: dbcrNamespace,
				},
				Spec: everestv1alpha1.DatabaseClusterRestoreSpec{
					DBClusterName: dbcrDbName,
				},
			},
			newDbcr: &everestv1alpha1.DatabaseClusterRestore{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dbcrName,
					Namespace: dbcrNamespace,
				},
				Spec: everestv1alpha1.DatabaseClusterRestoreSpec{
					DBClusterName: dbcrDbName,
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
		// already marked for deletion
		{
			name: "already marked to delete",
			dbcrToDelete: &everestv1alpha1.DatabaseClusterRestore{
				ObjectMeta: metav1.ObjectMeta{
					Name:              dbcrName,
					Namespace:         dbcrNamespace,
					DeletionTimestamp: &metav1.Time{},
				},
			},
		},
		// used by DatabaseCluster
		{
			name: "used by DatabaseCluster",
			dbcrToDelete: &everestv1alpha1.DatabaseClusterRestore{
				ObjectMeta: metav1.ObjectMeta{
					Name:       dbcrName,
					Namespace:  dbcrNamespace,
					Finalizers: []string{everestv1alpha1.InUseResourceFinalizer},
				},
			},
			wantErr: apierrors.NewForbidden(everestv1alpha1.GroupVersion.WithResource("databaseclusterrestore").GroupResource(),
				dbcrName,
				errDeleteInUse),
		},
		// not used by any DatabaseCluster
		{
			name: "not used by any DatabaseCluster",
			dbcrToDelete: &everestv1alpha1.DatabaseClusterRestore{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dbcrName,
					Namespace: dbcrNamespace,
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

			_, err := validator.ValidateDelete(context.TODO(), tc.dbcrToDelete)
			if tc.wantErr == nil {
				require.NoError(t, err)
				return
			}
			assert.Equal(t, tc.wantErr.Error(), err.Error())
		})
	}
}
