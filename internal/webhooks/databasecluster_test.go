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
package webhooks

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	everestv1alpha1 "github.com/percona/everest-operator/api/v1alpha1"
	"github.com/percona/everest-operator/internal/consts"
)

func TestCheckJSONKeyExists(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		obj      any
		key      string
		expected bool
	}{
		{
			obj: everestv1alpha1.DatabaseCluster{
				Spec: everestv1alpha1.DatabaseClusterSpec{
					Engine: everestv1alpha1.Engine{},
				},
			},
			key: ".spec.engine.type",
		},
		{
			obj: everestv1alpha1.DatabaseCluster{
				Spec: everestv1alpha1.DatabaseClusterSpec{
					Engine: everestv1alpha1.Engine{
						Type: everestv1alpha1.DatabaseEnginePSMDB,
					},
				},
			},
			key:      ".spec.engine.type",
			expected: true,
		},
		{
			obj: everestv1alpha1.DatabaseCluster{
				Spec: everestv1alpha1.DatabaseClusterSpec{
					Engine: everestv1alpha1.Engine{
						UserSecretsName: "my-user-secrets",
					},
				},
			},
			key:      ".spec.engine.userSecretsName",
			expected: true,
		},
		{
			obj: everestv1alpha1.DatabaseCluster{
				Spec: everestv1alpha1.DatabaseClusterSpec{
					Engine: everestv1alpha1.Engine{},
				},
			},
			key: ".spec.engine.userSecretsName",
		},
	}
	for _, tc := range testCases {
		exists, err := checkJSONKeyExists(tc.key, tc.obj)
		require.NoError(t, err)
		assert.Equal(t, tc.expected, exists, "Key existence check failed for key: %s", tc.key)
	}
}

func TestDatabaseClusterDefaulter(t *testing.T) {
	t.Parallel()
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = everestv1alpha1.AddToScheme(scheme)

	const (
		ns            = "test-ns"
		secretName    = "s3-creds"
		testAccessKey = "ZmFrZUFjY2Vzc0tleQ==" // base64 for "fakeAccessKey"
		testSecretKey = "ZmFrZVNlY3JldEtleQ==" //nolint:gosec // base64 for "fakeSecretKey"
	)

	apiObjects := []runtime.Object{
		&everestv1alpha1.DatabaseEngine{
			ObjectMeta: metav1.ObjectMeta{
				Name:      consts.PSMDBDeploymentName,
				Namespace: ns,
			},
			Spec: everestv1alpha1.DatabaseEngineSpec{
				Type: everestv1alpha1.DatabaseEnginePSMDB,
			},
			Status: everestv1alpha1.DatabaseEngineStatus{
				AvailableVersions: everestv1alpha1.Versions{
					Engine: everestv1alpha1.ComponentsMap{
						"8.0.8-3": {},
					},
				},
			},
		},
	}

	db := &everestv1alpha1.DatabaseCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: ns,
		},
		Spec: everestv1alpha1.DatabaseClusterSpec{
			Engine: everestv1alpha1.Engine{
				Type:    everestv1alpha1.DatabaseEnginePSMDB,
				Version: "8.0.8-3",
			},
			DataSource: &everestv1alpha1.DataSource{
				DataImport: &everestv1alpha1.DataImportJobTemplate{
					DataImporterName: "importer",
					Source: &everestv1alpha1.DataImportJobSource{
						S3: &everestv1alpha1.DataImportJobS3Source{
							Bucket:                "bucket",
							Region:                "region",
							EndpointURL:           "https://s3.example.com",
							CredentialsSecretName: secretName,
							AccessKeyID:           testAccessKey,
							SecretAccessKey:       testSecretKey,
						},
					},
				},
			},
		},
	}

	client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(apiObjects...).Build()
	defaulter := &DatabaseClusterDefaulter{Client: client}

	err := defaulter.Default(t.Context(), db)
	require.NoError(t, err)

	// Check that the credentials are removed from the spec
	assert.Empty(t, db.Spec.DataSource.DataImport.Source.S3.AccessKeyID)
	assert.Empty(t, db.Spec.DataSource.DataImport.Source.S3.SecretAccessKey)

	// Check that the secret was created and contains the expected data
	secret := &corev1.Secret{}
	err = client.Get(t.Context(), types.NamespacedName{Namespace: ns, Name: secretName}, secret)
	require.NoError(t, err)
	assert.Equal(t, testAccessKey, string(secret.Data[accessKeyIDSecretKey]))
	assert.Equal(t, testSecretKey, string(secret.Data[secretAccessKeySecretKey]))
}

func TestDatabaseClusterValidator(t *testing.T) {
	t.Parallel()
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = everestv1alpha1.AddToScheme(scheme)

	const userSecretName = "user-secret"
	t.Run("ValidateCreate", func(t *testing.T) {
		t.Parallel()
		ns := "test-ns"
		userSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      userSecretName,
				Namespace: ns,
			},
		}
		// Minimal OpenAPI schema for config with a required string field "foo"
		schema := map[string]any{
			"type":       "object",
			"properties": map[string]any{"foo": map[string]any{"type": "string"}},
			"required":   []string{"foo"},
		}
		schemaBytes, _ := json.Marshal(schema)
		var openAPISchema apiextensionsv1.JSONSchemaProps
		_ = json.Unmarshal(schemaBytes, &openAPISchema)

		dataImporter := &everestv1alpha1.DataImporter{
			ObjectMeta: metav1.ObjectMeta{
				Name: "importer",
			},
			Spec: everestv1alpha1.DataImporterSpec{
				SupportedEngines: everestv1alpha1.EngineList{everestv1alpha1.DatabaseEnginePSMDB},
				DatabaseClusterConstraints: everestv1alpha1.DataImporterDatabaseClusterConstraints{
					RequiredFields: []string{".spec.engine.userSecretsName"},
				},
				Config: everestv1alpha1.DataImporterConfig{
					OpenAPIV3Schema: &openAPISchema,
				},
			},
		}

		apiObjects := []runtime.Object{
			&everestv1alpha1.DatabaseEngine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      consts.PSMDBDeploymentName,
					Namespace: ns,
				},
				Spec: everestv1alpha1.DatabaseEngineSpec{
					Type: everestv1alpha1.DatabaseEnginePSMDB,
				},
				Status: everestv1alpha1.DatabaseEngineStatus{
					AvailableVersions: everestv1alpha1.Versions{
						Engine: everestv1alpha1.ComponentsMap{
							"8.0.8-3": {},
						},
					},
				},
			},
			&everestv1alpha1.DatabaseEngine{
				ObjectMeta: metav1.ObjectMeta{
					Name:      consts.PGDeploymentName,
					Namespace: ns,
				},
				Spec: everestv1alpha1.DatabaseEngineSpec{
					Type: everestv1alpha1.DatabaseEnginePostgresql,
				},
				Status: everestv1alpha1.DatabaseEngineStatus{
					AvailableVersions: everestv1alpha1.Versions{
						Engine: everestv1alpha1.ComponentsMap{
							"17.1": {},
						},
					},
				},
			},
		}

		testCases := []struct {
			name      string
			objects   []runtime.Object
			modify    func(*everestv1alpha1.DatabaseCluster)
			wantError string
		}{
			{
				name:    "invalid DBEngine version",
				objects: nil,
				modify: func(db *everestv1alpha1.DatabaseCluster) {
					db.Spec.Engine.Type = everestv1alpha1.DatabaseEnginePostgresql
					db.Spec.Engine.Version = "17.100"
				},
				wantError: "engine version 17.100 not available",
			},
			{
				name:    "missing user secret",
				objects: nil,
				modify: func(db *everestv1alpha1.DatabaseCluster) {
					db.Spec.Engine.UserSecretsName = userSecretName
				},
				wantError: "failed to get user secrets",
			},
			{
				name:    "present user secret",
				objects: []runtime.Object{userSecret},
				modify: func(db *everestv1alpha1.DatabaseCluster) {
					db.Spec.Engine.UserSecretsName = userSecretName
				},
				wantError: "",
			},
			{
				name:    "missing DataImporter",
				objects: []runtime.Object{userSecret},
				modify: func(db *everestv1alpha1.DatabaseCluster) {
					config := &runtime.RawExtension{Raw: []byte(`{"foo":"bar"}`)}
					db.Spec.DataSource = &everestv1alpha1.DataSource{
						DataImport: &everestv1alpha1.DataImportJobTemplate{
							DataImporterName: "importer",
							Config:           config,
						},
					}
				},
				wantError: "failed to get DataImporter",
			},
			{
				name:    "unsupported engine",
				objects: []runtime.Object{userSecret, dataImporter},
				modify: func(db *everestv1alpha1.DatabaseCluster) {
					config := &runtime.RawExtension{Raw: []byte(`{"foo":"bar"}`)}
					db.Spec.DataSource = &everestv1alpha1.DataSource{
						DataImport: &everestv1alpha1.DataImportJobTemplate{
							DataImporterName: "importer",
							Config:           config,
						},
					}
					db.Spec.Engine.Type = everestv1alpha1.DatabaseEnginePostgresql
					db.Spec.Engine.Version = "17.1"
				},
				wantError: "does not support engine",
			},
			{
				name:    "missing required field",
				objects: []runtime.Object{userSecret, dataImporter},
				modify: func(db *everestv1alpha1.DatabaseCluster) {
					config := &runtime.RawExtension{Raw: []byte(`{}`)}
					db.Spec.DataSource = &everestv1alpha1.DataSource{
						DataImport: &everestv1alpha1.DataImportJobTemplate{
							DataImporterName: "importer",
							Config:           config,
						},
					}
					db.Spec.Engine.Type = everestv1alpha1.DatabaseEnginePSMDB
				},
				wantError: "required field .spec.engine.userSecretsName is missing",
			},
			{
				name:    "valid config",
				objects: []runtime.Object{userSecret, dataImporter},
				modify: func(db *everestv1alpha1.DatabaseCluster) {
					config := &runtime.RawExtension{Raw: []byte(`{"foo":"bar"}`)}
					db.Spec.DataSource = &everestv1alpha1.DataSource{
						DataImport: &everestv1alpha1.DataImportJobTemplate{
							DataImporterName: "importer",
							Config:           config,
						},
					}
					db.Spec.Engine.Type = everestv1alpha1.DatabaseEnginePSMDB
					db.Spec.Engine.UserSecretsName = userSecretName
				},
				wantError: "",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()
				db := &everestv1alpha1.DatabaseCluster{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-db",
						Namespace: ns,
					},
					Spec: everestv1alpha1.DatabaseClusterSpec{
						Engine: everestv1alpha1.Engine{
							Type:    everestv1alpha1.DatabaseEnginePSMDB,
							Version: "8.0.8-3",
						},
					},
				}
				if tc.modify != nil {
					tc.modify(db)
				}
				// fake.NewClientBuilder().WithObjects expects client.Object, so we must ensure all objects are client.Object
				objs := make([]runtime.Object, 0, len(tc.objects)+len(apiObjects))
				objs = append(objs, apiObjects...)
				objs = append(objs, tc.objects...)
				client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...).Build()
				validator := &DatabaseClusterValidator{Client: client}
				_, err := validator.ValidateCreate(t.Context(), db)
				if tc.wantError == "" {
					assert.NoError(t, err)
				} else {
					assert.ErrorContains(t, err, tc.wantError)
				}
			})
		}
	})
}

func TestDatabaseClusterValidator_validatePXC84Memory(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		engineType    everestv1alpha1.EngineType
		engineVersion string
		memory        resource.Quantity
		isValid       bool
	}{
		{
			engineType:    everestv1alpha1.DatabaseEnginePXC,
			engineVersion: "8.4.5-5.1",
			memory:        resource.MustParse("3G"),
			isValid:       true,
		},
		{
			engineType:    everestv1alpha1.DatabaseEnginePXC,
			engineVersion: "8.4.5-5.1",
			memory:        resource.MustParse("4G"),
			isValid:       true,
		},
		{
			engineType:    everestv1alpha1.DatabaseEnginePXC,
			engineVersion: "8.4.5-5.1",
			memory:        resource.MustParse("64G"),
			isValid:       true,
		},
		{
			engineType:    everestv1alpha1.DatabaseEnginePXC,
			engineVersion: "8.4.5-5.1",
			memory:        resource.MustParse("1G"),
			isValid:       false,
		},
		{
			engineType:    everestv1alpha1.DatabaseEnginePXC,
			engineVersion: "8.4.5-5.1",
			memory:        resource.MustParse("2G"),
			isValid:       false,
		},
		{
			engineType:    everestv1alpha1.DatabaseEnginePXC,
			engineVersion: "8.0.42-33.1",
			memory:        resource.MustParse("2G"),
			isValid:       true,
		},
		{
			engineType:    everestv1alpha1.DatabaseEnginePXC,
			engineVersion: "8.0.42-33.1",
			memory:        resource.MustParse("1G"),
			isValid:       true,
		},
		{
			engineType:    everestv1alpha1.DatabaseEnginePXC,
			engineVersion: "8.0.42-33.1",
			memory:        resource.MustParse("64G"),
			isValid:       true,
		},
		{
			engineType: everestv1alpha1.DatabaseEnginePSMDB,
			isValid:    true,
		},
		{
			engineType: everestv1alpha1.DatabaseEnginePostgresql,
			isValid:    true,
		},
	}
	for _, tc := range testCases {
		db := &everestv1alpha1.DatabaseCluster{
			Spec: everestv1alpha1.DatabaseClusterSpec{
				Engine: everestv1alpha1.Engine{
					Type:    tc.engineType,
					Version: tc.engineVersion,
					Resources: everestv1alpha1.Resources{
						Memory: tc.memory,
					},
				},
			},
		}
		result := validatePXC84Memory(db)
		assert.Equal(t, tc.isValid, result)
	}
}

func TestDatabaseClusterValidator_validateEngineVersionChange(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		engineType everestv1alpha1.EngineType
		oldVersion string
		newVersion string
		wantError  error
	}{
		{
			name:       "invalid semver format",
			engineType: everestv1alpha1.DatabaseEnginePXC,
			oldVersion: "8.0.32-24",
			newVersion: "invalid",
			wantError:  errInvalidVersion,
		},
		{
			name:       "version downgrade attempt",
			engineType: everestv1alpha1.DatabaseEnginePXC,
			oldVersion: "8.0.32-24",
			newVersion: "8.0.31-24",
			wantError:  errDBEngineDowngrade,
		},
		{
			name:       "PG major version upgrade not allowed",
			engineType: everestv1alpha1.DatabaseEnginePostgresql,
			oldVersion: "14.10.0",
			newVersion: "15.0.0",
			wantError:  errDBEngineMajorVersionUpgrade,
		},
		{
			name:       "PSMDB major version upgrade allowed",
			engineType: everestv1alpha1.DatabaseEnginePSMDB,
			oldVersion: "6.0.5",
			newVersion: "7.0.0",
			wantError:  nil,
		},
		{
			name:       "PXC 8.0 to 8.4 upgrade not allowed",
			engineType: everestv1alpha1.DatabaseEnginePXC,
			oldVersion: "8.0.32-24",
			newVersion: "8.4.0-24",
			wantError:  errDBEnginePXC80To84Upgrade,
		},
		{
			name:       "non-sequential major version upgrade not allowed",
			engineType: everestv1alpha1.DatabaseEnginePSMDB,
			oldVersion: "5.0.0",
			newVersion: "7.0.0",
			wantError:  errDBEngineMajorUpgradeNotSeq,
		},
		{
			name:       "valid patch version upgrade",
			engineType: everestv1alpha1.DatabaseEnginePXC,
			oldVersion: "8.0.32-24",
			newVersion: "8.0.33-24",
			wantError:  nil,
		},
		{
			name:       "valid patch version upgrade",
			engineType: everestv1alpha1.DatabaseEnginePXC,
			oldVersion: "8.0.32-24",
			newVersion: "8.0.32-25",
			wantError:  nil,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := validateEngineVersionChange(tc.engineType, tc.oldVersion, tc.newVersion)
			if tc.wantError == nil {
				assert.NoError(t, err)
			} else {
				assert.Equal(t, tc.wantError, err)
			}
		})
	}
}
