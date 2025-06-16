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
package pg

import (
	"context"
	"testing"

	pgv2 "github.com/percona/percona-postgresql-operator/pkg/apis/pgv2.percona.com/v2"
	crunchyv1beta1 "github.com/percona/percona-postgresql-operator/pkg/apis/postgres-operator.crunchydata.com/v1beta1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/ini.v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestParseBackupPath(t *testing.T) {
	tests := []struct {
		name            string
		backupPath      string
		wantBackupName  string
		wantDBDirectory string
	}{
		{
			name:            "standard path",
			backupPath:      "/my-database/backup/db/backup-1",
			wantBackupName:  "backup-1",
			wantDBDirectory: "/my-database/",
		},
		{
			name:            "path with trailing slash",
			backupPath:      "/my-database/backup/db/backup-1/",
			wantBackupName:  "backup-1",
			wantDBDirectory: "/my-database/",
		},
		{
			name:            "path without leading slash",
			backupPath:      "my-database/backup/db/backup-1",
			wantBackupName:  "backup-1",
			wantDBDirectory: "/my-database/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backupName, dbDir := parseBackupPath(tt.backupPath)
			assert.Equal(t, tt.wantBackupName, backupName)
			assert.Equal(t, tt.wantDBDirectory, dbDir)
		})
	}
}

func TestGetRepoName(t *testing.T) {
	scheme := runtime.NewScheme()
	err := pgv2.AddToScheme(scheme)
	require.NoError(t, err)

	tests := []struct {
		name      string
		dbName    string
		namespace string
		pg        *pgv2.PerconaPGCluster
		want      string
		wantErr   bool
	}{
		{
			name:      "new repo with no existing repos",
			dbName:    "testdb",
			namespace: "default",
			pg: &pgv2.PerconaPGCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "testdb",
					Namespace: "default",
				},
				Spec: pgv2.PerconaPGClusterSpec{
					Backups: pgv2.Backups{
						PGBackRest: pgv2.PGBackRestArchive{
							Repos: []crunchyv1beta1.PGBackRestRepo{},
						},
					},
				},
			},
			want:    "repo1",
			wantErr: false,
		},
		{
			name:      "new repo with existing repos",
			dbName:    "testdb",
			namespace: "default",
			pg: &pgv2.PerconaPGCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "testdb",
					Namespace: "default",
				},
				Spec: pgv2.PerconaPGClusterSpec{
					Backups: pgv2.Backups{
						PGBackRest: pgv2.PGBackRestArchive{
							Repos: []crunchyv1beta1.PGBackRestRepo{
								{Name: "repo1"},
								{Name: "repo2"},
							},
						},
					},
				},
			},
			want:    "repo3",
			wantErr: false,
		},
		{
			name:      "new repo with existing repos",
			dbName:    "testdb",
			namespace: "default",
			pg: &pgv2.PerconaPGCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "testdb",
					Namespace: "default",
				},
				Spec: pgv2.PerconaPGClusterSpec{
					Backups: pgv2.Backups{
						PGBackRest: pgv2.PGBackRestArchive{
							Repos: []crunchyv1beta1.PGBackRestRepo{
								{Name: "repo1"},
								{Name: "repo2"},
								{Name: "repo3"},
								{Name: "repo4"},
							},
						},
					},
				},
			},
			want:    "repo3",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a fake client with the test PG cluster
			client := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(tt.pg).
				Build()

			got, err := getRepoName(context.Background(), client, tt.dbName, tt.namespace)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestPreparePGBackrestSecret(t *testing.T) {
	scheme := runtime.NewScheme()
	err := pgv2.AddToScheme(scheme)
	err = corev1.AddToScheme(scheme)
	require.NoError(t, err)

	var (
		repoName        = "repo1"
		secretName      = "test-pgbackrest-secret"
		accessKeyID     = "test-access-key-id"
		secretAccessKey = "test-secret-access-key"
		namespace       = "default"
	)

	initialSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"s3.conf": []byte(""),
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(initialSecret).
		Build()

	err = preparePGBackrestSecret(context.Background(), client, repoName, secretName,
		accessKeyID, secretAccessKey, namespace)
	require.NoError(t, err)

	updatedSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
		},
	}

	err = client.Get(context.Background(), types.NamespacedName{Namespace: namespace, Name: secretName}, updatedSecret)
	require.NoError(t, err)

	// note: we assert on StringData because we are using fakeClient.
	// In real scenario, Kubernetes will convert it to the Data field.
	s3Conf := updatedSecret.StringData["s3.conf"]
	assert.NotEmpty(t, s3Conf, "s3.conf should not be empty")

	s3Cfg, err := ini.LoadSources(ini.LoadOptions{}, []byte(s3Conf))
	require.NoError(t, err, "failed to parse s3.conf")
	assert.NotNil(t, s3Cfg, "s3.conf should be parsed successfully")

	global := s3Cfg.Section("global")
	assert.Equal(t, accessKeyID, global.Key(repoName+"-s3-key").String(), "s3 key should match")
	assert.Equal(t, secretAccessKey, global.Key(repoName+"-s3-key-secret").String(), "s3 secret key should match")
}

func TestPreparePGBackrestRepo(t *testing.T) {
	scheme := runtime.NewScheme()
	err := pgv2.AddToScheme(scheme)
	require.NoError(t, err)

	dbName := "testdb"
	namespace := "default"
	repoName := "repo1"
	secretName := "data-import-testdb"
	dbDirPath := "/my-database"
	uriStyle := "host"
	bucket := "test-bucket"
	endpoint := "s3.amazonaws.com"
	region := "us-east-1"

	pg := &pgv2.PerconaPGCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dbName,
			Namespace: namespace,
		},
		Spec: pgv2.PerconaPGClusterSpec{
			Backups: pgv2.Backups{
				PGBackRest: pgv2.PGBackRestArchive{
					Global: map[string]string{},
					Repos:  []crunchyv1beta1.PGBackRestRepo{},
				},
			},
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(pg).
		Build()

	err = preparePGBackrestRepo(context.Background(), client, repoName, secretName,
		dbDirPath, uriStyle, bucket, endpoint, region, dbName, namespace)
	require.NoError(t, err)

	updatedPG := &pgv2.PerconaPGCluster{}
	err = client.Get(context.Background(), types.NamespacedName{Namespace: namespace, Name: dbName}, updatedPG)
	require.NoError(t, err)

	// Check if the global configuration was updated
	assert.Equal(t, dbDirPath, updatedPG.Spec.Backups.PGBackRest.Global[repoName+"-path"])
	assert.Equal(t, uriStyle, updatedPG.Spec.Backups.PGBackRest.Global[repoName+"-s3-uri-style"])

	// Check if the repo was added
	require.Len(t, updatedPG.Spec.Backups.PGBackRest.Repos, 1)
	assert.Equal(t, repoName, updatedPG.Spec.Backups.PGBackRest.Repos[0].Name)
	assert.Equal(t, bucket, updatedPG.Spec.Backups.PGBackRest.Repos[0].S3.Bucket)
	assert.Equal(t, endpoint, updatedPG.Spec.Backups.PGBackRest.Repos[0].S3.Endpoint)
	assert.Equal(t, region, updatedPG.Spec.Backups.PGBackRest.Repos[0].S3.Region)
}
