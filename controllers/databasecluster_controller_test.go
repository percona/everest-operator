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

//nolint:dupl,goconst
package controllers

import (
	"context"
	"testing"

	crunchyv1beta1 "github.com/percona/percona-postgresql-operator/pkg/apis/postgres-operator.crunchydata.com/v1beta1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	everestv1alpha1 "github.com/percona/everest-operator/api/v1alpha1"
)

func TestGetOperatorVersion(t *testing.T) {
	t.Parallel()
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "percona-xtradb-cluster-operator",
			Namespace: "super-x",
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
	cl := fake.NewClientBuilder().WithObjects(deployment).Build()
	s := scheme.Scheme
	s.AddKnownTypes(everestv1alpha1.GroupVersion, &everestv1alpha1.DatabaseCluster{})
	r := &DatabaseClusterReconciler{Client: cl, Scheme: s}
	version, err := r.getOperatorVersion(context.TODO(), types.NamespacedName{
		Namespace: "super-x",
		Name:      "percona-xtradb-cluster-operator",
	})
	require.NoError(t, err)
	assert.Equal(t, "1.12.0", version.String())
	assert.NotEqual(t, "1.11.0", version.String())

	_, err = r.getOperatorVersion(context.TODO(), types.NamespacedName{
		Namespace: "non-existent",
		Name:      "percona-xtradb-cluster-operator",
	})
	require.Error(t, err)
}

func TestMergeMapSimple(t *testing.T) {
	t.Parallel()
	testDst := map[string]interface{}{
		"a": "apple",
		"b": "banana",
	}
	src := map[string]interface{}{
		"a": "avocado",
		"c": "cherry",
	}
	expDst := map[string]interface{}{
		"a": "avocado",
		"b": "banana",
		"c": "cherry",
	}
	err := mergeMap(testDst, src)
	require.NoError(t, err)
	assert.Equal(t, expDst, testDst)
}

func TestMergeMapNested(t *testing.T) {
	t.Parallel()
	testDst := map[string]interface{}{
		"a": "apple",
		"b": "banana",
		"dry": map[string]interface{}{
			"a": "almond",
			"p": "peanut",
		},
	}
	src := map[string]interface{}{
		"a": "avocado",
		"c": "cherry",
		"dry": map[string]interface{}{
			"c": "cashew",
			"p": "pecan",
		},
		"vegetables": map[string]interface{}{
			"a": "aspargus",
			"b": "beet",
		},
	}
	expDst := map[string]interface{}{
		"a": "avocado",
		"b": "banana",
		"c": "cherry",
		"dry": map[string]interface{}{
			"a": "almond",
			"c": "cashew",
			"p": "pecan",
		},
		"vegetables": map[string]interface{}{
			"a": "aspargus",
			"b": "beet",
		},
	}
	err := mergeMap(testDst, src)
	require.NoError(t, err)
	assert.Equal(t, expDst, testDst)
}

func TestMergeMapError(t *testing.T) {
	t.Parallel()
	testDst := map[string]interface{}{
		"dry": map[string]interface{}{
			"vegetables": map[string]interface{}{
				"a": 1,
			},
		},
	}
	src := map[string]interface{}{
		"dry": map[string]interface{}{
			"vegetables": map[string]interface{}{
				"a": "avocado",
			},
		},
	}
	err := mergeMap(testDst, src)
	require.Error(t, err)
}

func TestReconcilePGBackRestReposEmptyAddRequest(t *testing.T) {
	t.Parallel()
	testRepos := []crunchyv1beta1.PGBackRestRepo{}
	testBackupSchedules := []everestv1alpha1.BackupSchedule{}
	testBackupRequests := []everestv1alpha1.DatabaseClusterBackup{
		{
			Spec: everestv1alpha1.DatabaseClusterBackupSpec{
				BackupStorageName: "backupStorage1",
			},
		},
	}
	testBackupStorages := map[string]everestv1alpha1.BackupStorageSpec{
		"backupStorage1": {
			Type:        everestv1alpha1.BackupStorageTypeS3,
			Bucket:      "bucket1",
			Region:      "region1",
			EndpointURL: "endpoint1",
		},
	}
	testBackupStoragesSecrets := map[string]*corev1.Secret{
		"backupStorage1": {
			Data: map[string][]byte{
				"AWS_ACCESS_KEY_ID":     []byte("SomeAccessKeyID"),
				"AWS_SECRET_ACCESS_KEY": []byte("SomeSecretAccessKey"),
			},
		},
	}
	testEngineStorageSize, _ := resource.ParseQuantity("15G")
	testEngineStorageClass := "someSC"
	testEngineStorage := everestv1alpha1.Storage{
		Size:  testEngineStorageSize,
		Class: &testEngineStorageClass,
	}
	expRepos := []crunchyv1beta1.PGBackRestRepo{
		{
			Name: "repo1",
			Volume: &crunchyv1beta1.RepoPVC{
				VolumeClaimSpec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteOnce,
					},
					StorageClassName: &testEngineStorageClass,
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: testEngineStorageSize,
						},
					},
				},
			},
		},
		{
			Name: "repo2",
			S3: &crunchyv1beta1.RepoS3{
				Bucket:   "bucket1",
				Region:   "region1",
				Endpoint: "endpoint1",
			},
		},
	}

	repos, _, _, err := reconcilePGBackRestRepos(
		testRepos,
		testBackupSchedules,
		testBackupRequests,
		testBackupStorages,
		testBackupStoragesSecrets,
		testEngineStorage,
		&everestv1alpha1.DatabaseCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test",
				UID:  "123",
			},
		},
	)
	require.NoError(t, err)
	assert.Equal(t, expRepos, repos)
}

func TestReconcilePGBackRestReposEmptyAddSchedule(t *testing.T) {
	t.Parallel()
	testRepos := []crunchyv1beta1.PGBackRestRepo{}
	testSchedule := "0 0 * * *"
	testBackupSchedules := []everestv1alpha1.BackupSchedule{
		{
			Enabled:           true,
			Name:              "schedule1",
			Schedule:          testSchedule,
			BackupStorageName: "backupStorage1",
		},
	}
	testBackupRequests := []everestv1alpha1.DatabaseClusterBackup{}
	testBackupStorages := map[string]everestv1alpha1.BackupStorageSpec{
		"backupStorage1": {
			Type:        everestv1alpha1.BackupStorageTypeS3,
			Bucket:      "bucket1",
			Region:      "region1",
			EndpointURL: "endpoint1",
		},
	}
	testBackupStoragesSecrets := map[string]*corev1.Secret{
		"backupStorage1": {
			Data: map[string][]byte{
				"AWS_ACCESS_KEY_ID":     []byte("SomeAccessKeyID"),
				"AWS_SECRET_ACCESS_KEY": []byte("SomeSecretAccessKey"),
			},
		},
	}
	testEngineStorageSize, _ := resource.ParseQuantity("15G")
	testEngineStorageClass := "someSC"
	testEngineStorage := everestv1alpha1.Storage{
		Size:  testEngineStorageSize,
		Class: &testEngineStorageClass,
	}
	expRepos := []crunchyv1beta1.PGBackRestRepo{
		{
			Name: "repo1",
			Volume: &crunchyv1beta1.RepoPVC{
				VolumeClaimSpec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteOnce,
					},
					StorageClassName: &testEngineStorageClass,
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: testEngineStorageSize,
						},
					},
				},
			},
		},
		{
			Name: "repo2",
			S3: &crunchyv1beta1.RepoS3{
				Bucket:   "bucket1",
				Region:   "region1",
				Endpoint: "endpoint1",
			},
			BackupSchedules: &crunchyv1beta1.PGBackRestBackupSchedules{
				Full: &testSchedule,
			},
		},
	}

	repos, _, _, err := reconcilePGBackRestRepos(
		testRepos,
		testBackupSchedules,
		testBackupRequests,
		testBackupStorages,
		testBackupStoragesSecrets,
		testEngineStorage,
		&everestv1alpha1.DatabaseCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test",
				UID:  "123",
			},
		},
	)
	require.NoError(t, err)
	assert.Equal(t, expRepos, repos)
}

func TestReconcilePGBackRestReposSameStorageOneRequestAddRequest(t *testing.T) {
	t.Parallel()
	testEngineStorageSize, _ := resource.ParseQuantity("15G")
	testEngineStorageClass := "someSC"
	testRepos := []crunchyv1beta1.PGBackRestRepo{
		{
			Name: "repo1",
			Volume: &crunchyv1beta1.RepoPVC{
				VolumeClaimSpec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteOnce,
					},
					StorageClassName: &testEngineStorageClass,
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: testEngineStorageSize,
						},
					},
				},
			},
		},
		{
			Name: "repo2",
			S3: &crunchyv1beta1.RepoS3{
				Bucket:   "bucket1",
				Region:   "region1",
				Endpoint: "endpoint1",
			},
		},
	}
	testBackupSchedules := []everestv1alpha1.BackupSchedule{}
	testBackupRequests := []everestv1alpha1.DatabaseClusterBackup{
		{
			Spec: everestv1alpha1.DatabaseClusterBackupSpec{
				BackupStorageName: "backupStorage1",
			},
		},
		{
			Spec: everestv1alpha1.DatabaseClusterBackupSpec{
				BackupStorageName: "backupStorage1",
			},
		},
	}
	testBackupStorages := map[string]everestv1alpha1.BackupStorageSpec{
		"backupStorage1": {
			Type:        everestv1alpha1.BackupStorageTypeS3,
			Bucket:      "bucket1",
			Region:      "region1",
			EndpointURL: "endpoint1",
		},
	}
	testBackupStoragesSecrets := map[string]*corev1.Secret{
		"backupStorage1": {
			Data: map[string][]byte{
				"AWS_ACCESS_KEY_ID":     []byte("SomeAccessKeyID"),
				"AWS_SECRET_ACCESS_KEY": []byte("SomeSecretAccessKey"),
			},
		},
	}
	testEngineStorage := everestv1alpha1.Storage{
		Size:  testEngineStorageSize,
		Class: &testEngineStorageClass,
	}
	expRepos := []crunchyv1beta1.PGBackRestRepo{
		{
			Name: "repo1",
			Volume: &crunchyv1beta1.RepoPVC{
				VolumeClaimSpec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteOnce,
					},
					StorageClassName: &testEngineStorageClass,
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: testEngineStorageSize,
						},
					},
				},
			},
		},
		{
			Name: "repo2",
			S3: &crunchyv1beta1.RepoS3{
				Bucket:   "bucket1",
				Region:   "region1",
				Endpoint: "endpoint1",
			},
		},
	}

	repos, _, _, err := reconcilePGBackRestRepos(
		testRepos,
		testBackupSchedules,
		testBackupRequests,
		testBackupStorages,
		testBackupStoragesSecrets,
		testEngineStorage,
		&everestv1alpha1.DatabaseCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test",
				UID:  "123",
			},
		},
	)
	require.NoError(t, err)
	assert.Equal(t, expRepos, repos)
}

func TestReconcilePGBackRestReposSameStorageOneRequestAddSchedule(t *testing.T) {
	t.Parallel()
	testEngineStorageSize, _ := resource.ParseQuantity("15G")
	testEngineStorageClass := "someSC"
	testRepos := []crunchyv1beta1.PGBackRestRepo{
		{
			Name: "repo1",
			Volume: &crunchyv1beta1.RepoPVC{
				VolumeClaimSpec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteOnce,
					},
					StorageClassName: &testEngineStorageClass,
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: testEngineStorageSize,
						},
					},
				},
			},
		},
		{
			Name: "repo2",
			S3: &crunchyv1beta1.RepoS3{
				Bucket:   "bucket1",
				Region:   "region1",
				Endpoint: "endpoint1",
			},
		},
	}
	testSchedule := "0 0 * * *"
	testBackupSchedules := []everestv1alpha1.BackupSchedule{
		{
			Enabled:           true,
			Name:              "schedule1",
			Schedule:          testSchedule,
			BackupStorageName: "backupStorage1",
		},
	}
	testBackupRequests := []everestv1alpha1.DatabaseClusterBackup{
		{
			Spec: everestv1alpha1.DatabaseClusterBackupSpec{
				BackupStorageName: "backupStorage1",
			},
		},
	}
	testBackupStorages := map[string]everestv1alpha1.BackupStorageSpec{
		"backupStorage1": {
			Type:        everestv1alpha1.BackupStorageTypeS3,
			Bucket:      "bucket1",
			Region:      "region1",
			EndpointURL: "endpoint1",
		},
	}
	testBackupStoragesSecrets := map[string]*corev1.Secret{
		"backupStorage1": {
			Data: map[string][]byte{
				"AWS_ACCESS_KEY_ID":     []byte("SomeAccessKeyID"),
				"AWS_SECRET_ACCESS_KEY": []byte("SomeSecretAccessKey"),
			},
		},
	}
	testEngineStorage := everestv1alpha1.Storage{
		Size:  testEngineStorageSize,
		Class: &testEngineStorageClass,
	}
	expRepos := []crunchyv1beta1.PGBackRestRepo{
		{
			Name: "repo1",
			Volume: &crunchyv1beta1.RepoPVC{
				VolumeClaimSpec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteOnce,
					},
					StorageClassName: &testEngineStorageClass,
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: testEngineStorageSize,
						},
					},
				},
			},
		},
		{
			Name: "repo2",
			S3: &crunchyv1beta1.RepoS3{
				Bucket:   "bucket1",
				Region:   "region1",
				Endpoint: "endpoint1",
			},
			BackupSchedules: &crunchyv1beta1.PGBackRestBackupSchedules{
				Full: &testSchedule,
			},
		},
	}

	repos, _, _, err := reconcilePGBackRestRepos(
		testRepos,
		testBackupSchedules,
		testBackupRequests,
		testBackupStorages,
		testBackupStoragesSecrets,
		testEngineStorage,
		&everestv1alpha1.DatabaseCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test",
				UID:  "123",
			},
		},
	)
	require.NoError(t, err)
	assert.Equal(t, expRepos, repos)
}

func TestReconcilePGBackRestReposSameStorageOneScheduleAddRequest(t *testing.T) {
	t.Parallel()
	testSchedule := "0 0 * * *"
	testEngineStorageSize, _ := resource.ParseQuantity("15G")
	testEngineStorageClass := "someSC"
	testRepos := []crunchyv1beta1.PGBackRestRepo{
		{
			Name: "repo1",
			Volume: &crunchyv1beta1.RepoPVC{
				VolumeClaimSpec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteOnce,
					},
					StorageClassName: &testEngineStorageClass,
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: testEngineStorageSize,
						},
					},
				},
			},
		},
		{
			Name: "repo2",
			S3: &crunchyv1beta1.RepoS3{
				Bucket:   "bucket1",
				Region:   "region1",
				Endpoint: "endpoint1",
			},
			BackupSchedules: &crunchyv1beta1.PGBackRestBackupSchedules{
				Full: &testSchedule,
			},
		},
	}
	testBackupSchedules := []everestv1alpha1.BackupSchedule{
		{
			Enabled:           true,
			Name:              "schedule1",
			Schedule:          testSchedule,
			BackupStorageName: "backupStorage1",
		},
	}
	testBackupRequests := []everestv1alpha1.DatabaseClusterBackup{
		{
			Spec: everestv1alpha1.DatabaseClusterBackupSpec{
				BackupStorageName: "backupStorage1",
			},
		},
	}
	testBackupStorages := map[string]everestv1alpha1.BackupStorageSpec{
		"backupStorage1": {
			Type:        everestv1alpha1.BackupStorageTypeS3,
			Bucket:      "bucket1",
			Region:      "region1",
			EndpointURL: "endpoint1",
		},
	}
	testBackupStoragesSecrets := map[string]*corev1.Secret{
		"backupStorage1": {
			Data: map[string][]byte{
				"AWS_ACCESS_KEY_ID":     []byte("SomeAccessKeyID"),
				"AWS_SECRET_ACCESS_KEY": []byte("SomeSecretAccessKey"),
			},
		},
	}
	testEngineStorage := everestv1alpha1.Storage{
		Size:  testEngineStorageSize,
		Class: &testEngineStorageClass,
	}
	expRepos := []crunchyv1beta1.PGBackRestRepo{
		{
			Name: "repo1",
			Volume: &crunchyv1beta1.RepoPVC{
				VolumeClaimSpec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteOnce,
					},
					StorageClassName: &testEngineStorageClass,
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: testEngineStorageSize,
						},
					},
				},
			},
		},
		{
			Name: "repo2",
			S3: &crunchyv1beta1.RepoS3{
				Bucket:   "bucket1",
				Region:   "region1",
				Endpoint: "endpoint1",
			},
			BackupSchedules: &crunchyv1beta1.PGBackRestBackupSchedules{
				Full: &testSchedule,
			},
		},
	}

	repos, _, _, err := reconcilePGBackRestRepos(
		testRepos,
		testBackupSchedules,
		testBackupRequests,
		testBackupStorages,
		testBackupStoragesSecrets,
		testEngineStorage,
		&everestv1alpha1.DatabaseCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test",
				UID:  "123",
			},
		},
	)
	require.NoError(t, err)
	assert.Equal(t, expRepos, repos)
}

func TestReconcilePGBackRestReposSameStorageOneScheduleAddSchedule(t *testing.T) {
	t.Parallel()
	testSchedule1 := "0 0 * * *"
	testSchedule2 := "0 1 * * *"
	testEngineStorageSize, _ := resource.ParseQuantity("15G")
	testEngineStorageClass := "someSC"
	testRepos := []crunchyv1beta1.PGBackRestRepo{
		{
			Name: "repo1",
			Volume: &crunchyv1beta1.RepoPVC{
				VolumeClaimSpec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteOnce,
					},
					StorageClassName: &testEngineStorageClass,
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: testEngineStorageSize,
						},
					},
				},
			},
		},
		{
			Name: "repo2",
			S3: &crunchyv1beta1.RepoS3{
				Bucket:   "bucket1",
				Region:   "region1",
				Endpoint: "endpoint1",
			},
			BackupSchedules: &crunchyv1beta1.PGBackRestBackupSchedules{
				Full: &testSchedule1,
			},
		},
	}
	testBackupSchedules := []everestv1alpha1.BackupSchedule{
		{
			Enabled:           true,
			Name:              "schedule1",
			Schedule:          testSchedule1,
			BackupStorageName: "backupStorage1",
		},
		{
			Enabled:           true,
			Name:              "schedule2",
			Schedule:          testSchedule2,
			BackupStorageName: "backupStorage1",
		},
	}
	testBackupRequests := []everestv1alpha1.DatabaseClusterBackup{}
	testBackupStorages := map[string]everestv1alpha1.BackupStorageSpec{
		"backupStorage1": {
			Type:        everestv1alpha1.BackupStorageTypeS3,
			Bucket:      "bucket1",
			Region:      "region1",
			EndpointURL: "endpoint1",
		},
	}
	testBackupStoragesSecrets := map[string]*corev1.Secret{
		"backupStorage1": {
			Data: map[string][]byte{
				"AWS_ACCESS_KEY_ID":     []byte("SomeAccessKeyID"),
				"AWS_SECRET_ACCESS_KEY": []byte("SomeSecretAccessKey"),
			},
		},
	}
	testEngineStorage := everestv1alpha1.Storage{
		Size:  testEngineStorageSize,
		Class: &testEngineStorageClass,
	}
	expRepos := []crunchyv1beta1.PGBackRestRepo{
		{
			Name: "repo1",
			Volume: &crunchyv1beta1.RepoPVC{
				VolumeClaimSpec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteOnce,
					},
					StorageClassName: &testEngineStorageClass,
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: testEngineStorageSize,
						},
					},
				},
			},
		},
		{
			Name: "repo2",
			S3: &crunchyv1beta1.RepoS3{
				Bucket:   "bucket1",
				Region:   "region1",
				Endpoint: "endpoint1",
			},
			BackupSchedules: &crunchyv1beta1.PGBackRestBackupSchedules{
				Full: &testSchedule1,
			},
		},
		{
			Name: "repo3",
			S3: &crunchyv1beta1.RepoS3{
				Bucket:   "bucket1",
				Region:   "region1",
				Endpoint: "endpoint1",
			},
			BackupSchedules: &crunchyv1beta1.PGBackRestBackupSchedules{
				Full: &testSchedule2,
			},
		},
	}

	repos, _, _, err := reconcilePGBackRestRepos(
		testRepos,
		testBackupSchedules,
		testBackupRequests,
		testBackupStorages,
		testBackupStoragesSecrets,
		testEngineStorage,
		&everestv1alpha1.DatabaseCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test",
				UID:  "123",
			},
		},
	)
	require.NoError(t, err)
	assert.Equal(t, expRepos, repos)
}

func TestReconcilePGBackRestReposDifferentStorageOneScheduleAddRequest(t *testing.T) {
	t.Parallel()
	testSchedule := "0 0 * * *"
	testEngineStorageSize, _ := resource.ParseQuantity("15G")
	testEngineStorageClass := "someSC"
	testRepos := []crunchyv1beta1.PGBackRestRepo{
		{
			Name: "repo1",
			Volume: &crunchyv1beta1.RepoPVC{
				VolumeClaimSpec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteOnce,
					},
					StorageClassName: &testEngineStorageClass,
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: testEngineStorageSize,
						},
					},
				},
			},
		},
		{
			Name: "repo2",
			S3: &crunchyv1beta1.RepoS3{
				Bucket:   "bucket1",
				Region:   "region1",
				Endpoint: "endpoint1",
			},
			BackupSchedules: &crunchyv1beta1.PGBackRestBackupSchedules{
				Full: &testSchedule,
			},
		},
	}
	testBackupSchedules := []everestv1alpha1.BackupSchedule{
		{
			Enabled:           true,
			Name:              "schedule1",
			Schedule:          testSchedule,
			BackupStorageName: "backupStorage1",
		},
	}
	testBackupRequests := []everestv1alpha1.DatabaseClusterBackup{
		{
			Spec: everestv1alpha1.DatabaseClusterBackupSpec{
				BackupStorageName: "backupStorage2",
			},
		},
	}
	testBackupStorages := map[string]everestv1alpha1.BackupStorageSpec{
		"backupStorage1": {
			Type:        everestv1alpha1.BackupStorageTypeS3,
			Bucket:      "bucket1",
			Region:      "region1",
			EndpointURL: "endpoint1",
		},
		"backupStorage2": {
			Type:        everestv1alpha1.BackupStorageTypeS3,
			Bucket:      "bucket2",
			Region:      "region2",
			EndpointURL: "endpoint2",
		},
	}
	testBackupStoragesSecrets := map[string]*corev1.Secret{
		"backupStorage1": {
			Data: map[string][]byte{
				"AWS_ACCESS_KEY_ID":     []byte("SomeAccessKeyID"),
				"AWS_SECRET_ACCESS_KEY": []byte("SomeSecretAccessKey"),
			},
		},
		"backupStorage2": {
			Data: map[string][]byte{
				"AWS_ACCESS_KEY_ID":     []byte("SomeAccessKeyID"),
				"AWS_SECRET_ACCESS_KEY": []byte("SomeSecretAccessKey"),
			},
		},
	}
	testEngineStorage := everestv1alpha1.Storage{
		Size:  testEngineStorageSize,
		Class: &testEngineStorageClass,
	}
	expRepos := []crunchyv1beta1.PGBackRestRepo{
		{
			Name: "repo1",
			Volume: &crunchyv1beta1.RepoPVC{
				VolumeClaimSpec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteOnce,
					},
					StorageClassName: &testEngineStorageClass,
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: testEngineStorageSize,
						},
					},
				},
			},
		},
		{
			Name: "repo2",
			S3: &crunchyv1beta1.RepoS3{
				Bucket:   "bucket1",
				Region:   "region1",
				Endpoint: "endpoint1",
			},
			BackupSchedules: &crunchyv1beta1.PGBackRestBackupSchedules{
				Full: &testSchedule,
			},
		},
		{
			Name: "repo3",
			S3: &crunchyv1beta1.RepoS3{
				Bucket:   "bucket2",
				Region:   "region2",
				Endpoint: "endpoint2",
			},
		},
	}

	repos, _, _, err := reconcilePGBackRestRepos(
		testRepos,
		testBackupSchedules,
		testBackupRequests,
		testBackupStorages,
		testBackupStoragesSecrets,
		testEngineStorage,
		&everestv1alpha1.DatabaseCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test",
				UID:  "123",
			},
		},
	)
	require.NoError(t, err)
	assert.Equal(t, expRepos, repos)
}

func TestReconcilePGBackRestReposDifferentStorageOneScheduleAddSchedule(t *testing.T) {
	t.Parallel()
	testSchedule1 := "0 0 * * *"
	testSchedule2 := "0 1 * * *"
	testEngineStorageSize, _ := resource.ParseQuantity("15G")
	testEngineStorageClass := "someSC"
	testRepos := []crunchyv1beta1.PGBackRestRepo{
		{
			Name: "repo1",
			Volume: &crunchyv1beta1.RepoPVC{
				VolumeClaimSpec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteOnce,
					},
					StorageClassName: &testEngineStorageClass,
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: testEngineStorageSize,
						},
					},
				},
			},
		},
		{
			Name: "repo2",
			S3: &crunchyv1beta1.RepoS3{
				Bucket:   "bucket1",
				Region:   "region1",
				Endpoint: "endpoint1",
			},
			BackupSchedules: &crunchyv1beta1.PGBackRestBackupSchedules{
				Full: &testSchedule1,
			},
		},
	}
	testBackupSchedules := []everestv1alpha1.BackupSchedule{
		{
			Enabled:           true,
			Name:              "schedule1",
			Schedule:          testSchedule1,
			BackupStorageName: "backupStorage1",
		},
		{
			Enabled:           true,
			Name:              "schedule2",
			Schedule:          testSchedule2,
			BackupStorageName: "backupStorage2",
		},
	}
	testBackupRequests := []everestv1alpha1.DatabaseClusterBackup{}
	testBackupStorages := map[string]everestv1alpha1.BackupStorageSpec{
		"backupStorage1": {
			Type:        everestv1alpha1.BackupStorageTypeS3,
			Bucket:      "bucket1",
			Region:      "region1",
			EndpointURL: "endpoint1",
		},
		"backupStorage2": {
			Type:        everestv1alpha1.BackupStorageTypeS3,
			Bucket:      "bucket2",
			Region:      "region2",
			EndpointURL: "endpoint2",
		},
	}
	testBackupStoragesSecrets := map[string]*corev1.Secret{
		"backupStorage1": {
			Data: map[string][]byte{
				"AWS_ACCESS_KEY_ID":     []byte("SomeAccessKeyID"),
				"AWS_SECRET_ACCESS_KEY": []byte("SomeSecretAccessKey"),
			},
		},
		"backupStorage2": {
			Data: map[string][]byte{
				"AWS_ACCESS_KEY_ID":     []byte("SomeAccessKeyID"),
				"AWS_SECRET_ACCESS_KEY": []byte("SomeSecretAccessKey"),
			},
		},
	}
	testEngineStorage := everestv1alpha1.Storage{
		Size:  testEngineStorageSize,
		Class: &testEngineStorageClass,
	}
	expRepos := []crunchyv1beta1.PGBackRestRepo{
		{
			Name: "repo1",
			Volume: &crunchyv1beta1.RepoPVC{
				VolumeClaimSpec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteOnce,
					},
					StorageClassName: &testEngineStorageClass,
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: testEngineStorageSize,
						},
					},
				},
			},
		},
		{
			Name: "repo2",
			S3: &crunchyv1beta1.RepoS3{
				Bucket:   "bucket1",
				Region:   "region1",
				Endpoint: "endpoint1",
			},
			BackupSchedules: &crunchyv1beta1.PGBackRestBackupSchedules{
				Full: &testSchedule1,
			},
		},
		{
			Name: "repo3",
			S3: &crunchyv1beta1.RepoS3{
				Bucket:   "bucket2",
				Region:   "region2",
				Endpoint: "endpoint2",
			},
			BackupSchedules: &crunchyv1beta1.PGBackRestBackupSchedules{
				Full: &testSchedule2,
			},
		},
	}

	repos, _, _, err := reconcilePGBackRestRepos(
		testRepos,
		testBackupSchedules,
		testBackupRequests,
		testBackupStorages,
		testBackupStoragesSecrets,
		testEngineStorage,
		&everestv1alpha1.DatabaseCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test",
				UID:  "123",
			},
		},
	)
	require.NoError(t, err)
	assert.Equal(t, expRepos, repos)
}

func TestReconcilePGBackRestReposDifferentStorageOneScheduleAddScheduleNoOrder(t *testing.T) {
	t.Parallel()
	testSchedule1 := "0 0 * * *"
	testSchedule2 := "0 1 * * *"
	testEngineStorageSize, _ := resource.ParseQuantity("15G")
	testEngineStorageClass := "someSC"
	testRepos := []crunchyv1beta1.PGBackRestRepo{
		{
			Name: "repo1",
			Volume: &crunchyv1beta1.RepoPVC{
				VolumeClaimSpec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteOnce,
					},
					StorageClassName: &testEngineStorageClass,
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: testEngineStorageSize,
						},
					},
				},
			},
		},
		{
			Name: "repo3",
			S3: &crunchyv1beta1.RepoS3{
				Bucket:   "bucket2",
				Region:   "region2",
				Endpoint: "endpoint2",
			},
			BackupSchedules: &crunchyv1beta1.PGBackRestBackupSchedules{
				Full: &testSchedule2,
			},
		},
	}
	testBackupSchedules := []everestv1alpha1.BackupSchedule{
		{
			Enabled:           true,
			Name:              "schedule1",
			Schedule:          testSchedule1,
			BackupStorageName: "backupStorage1",
		},
		{
			Enabled:           true,
			Name:              "schedule2",
			Schedule:          testSchedule2,
			BackupStorageName: "backupStorage2",
		},
	}
	testBackupRequests := []everestv1alpha1.DatabaseClusterBackup{}
	testBackupStorages := map[string]everestv1alpha1.BackupStorageSpec{
		"backupStorage1": {
			Type:        everestv1alpha1.BackupStorageTypeS3,
			Bucket:      "bucket1",
			Region:      "region1",
			EndpointURL: "endpoint1",
		},
		"backupStorage2": {
			Type:        everestv1alpha1.BackupStorageTypeS3,
			Bucket:      "bucket2",
			Region:      "region2",
			EndpointURL: "endpoint2",
		},
	}
	testBackupStoragesSecrets := map[string]*corev1.Secret{
		"backupStorage1": {
			Data: map[string][]byte{
				"AWS_ACCESS_KEY_ID":     []byte("SomeAccessKeyID"),
				"AWS_SECRET_ACCESS_KEY": []byte("SomeSecretAccessKey"),
			},
		},
		"backupStorage2": {
			Data: map[string][]byte{
				"AWS_ACCESS_KEY_ID":     []byte("SomeAccessKeyID"),
				"AWS_SECRET_ACCESS_KEY": []byte("SomeSecretAccessKey"),
			},
		},
	}
	testEngineStorage := everestv1alpha1.Storage{
		Size:  testEngineStorageSize,
		Class: &testEngineStorageClass,
	}
	expRepos := []crunchyv1beta1.PGBackRestRepo{
		{
			Name: "repo1",
			Volume: &crunchyv1beta1.RepoPVC{
				VolumeClaimSpec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteOnce,
					},
					StorageClassName: &testEngineStorageClass,
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: testEngineStorageSize,
						},
					},
				},
			},
		},
		{
			Name: "repo3",
			S3: &crunchyv1beta1.RepoS3{
				Bucket:   "bucket2",
				Region:   "region2",
				Endpoint: "endpoint2",
			},
			BackupSchedules: &crunchyv1beta1.PGBackRestBackupSchedules{
				Full: &testSchedule2,
			},
		},
		{
			Name: "repo2",
			S3: &crunchyv1beta1.RepoS3{
				Bucket:   "bucket1",
				Region:   "region1",
				Endpoint: "endpoint1",
			},
			BackupSchedules: &crunchyv1beta1.PGBackRestBackupSchedules{
				Full: &testSchedule1,
			},
		},
	}

	repos, _, _, err := reconcilePGBackRestRepos(
		testRepos,
		testBackupSchedules,
		testBackupRequests,
		testBackupStorages,
		testBackupStoragesSecrets,
		testEngineStorage,
		&everestv1alpha1.DatabaseCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test",
				UID:  "123",
			},
		},
	)
	require.NoError(t, err)
	assert.Equal(t, expRepos, repos)
}

func TestReconcilePGBackRestReposDifferentStorageOneScheduleAddRequestNoOrder(t *testing.T) {
	t.Parallel()
	testSchedule2 := "0 1 * * *"
	testEngineStorageSize, _ := resource.ParseQuantity("15G")
	testEngineStorageClass := "someSC"
	testRepos := []crunchyv1beta1.PGBackRestRepo{
		{
			Name: "repo1",
			Volume: &crunchyv1beta1.RepoPVC{
				VolumeClaimSpec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteOnce,
					},
					StorageClassName: &testEngineStorageClass,
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: testEngineStorageSize,
						},
					},
				},
			},
		},
		{
			Name: "repo3",
			S3: &crunchyv1beta1.RepoS3{
				Bucket:   "bucket2",
				Region:   "region2",
				Endpoint: "endpoint2",
			},
			BackupSchedules: &crunchyv1beta1.PGBackRestBackupSchedules{
				Full: &testSchedule2,
			},
		},
	}
	testBackupSchedules := []everestv1alpha1.BackupSchedule{
		{
			Enabled:           true,
			Name:              "schedule2",
			Schedule:          testSchedule2,
			BackupStorageName: "backupStorage2",
		},
	}
	testBackupRequests := []everestv1alpha1.DatabaseClusterBackup{
		{
			Spec: everestv1alpha1.DatabaseClusterBackupSpec{
				BackupStorageName: "backupStorage1",
			},
		},
	}
	testBackupStorages := map[string]everestv1alpha1.BackupStorageSpec{
		"backupStorage1": {
			Type:        everestv1alpha1.BackupStorageTypeS3,
			Bucket:      "bucket1",
			Region:      "region1",
			EndpointURL: "endpoint1",
		},
		"backupStorage2": {
			Type:        everestv1alpha1.BackupStorageTypeS3,
			Bucket:      "bucket2",
			Region:      "region2",
			EndpointURL: "endpoint2",
		},
	}
	testBackupStoragesSecrets := map[string]*corev1.Secret{
		"backupStorage1": {
			Data: map[string][]byte{
				"AWS_ACCESS_KEY_ID":     []byte("SomeAccessKeyID"),
				"AWS_SECRET_ACCESS_KEY": []byte("SomeSecretAccessKey"),
			},
		},
		"backupStorage2": {
			Data: map[string][]byte{
				"AWS_ACCESS_KEY_ID":     []byte("SomeAccessKeyID"),
				"AWS_SECRET_ACCESS_KEY": []byte("SomeSecretAccessKey"),
			},
		},
	}
	testEngineStorage := everestv1alpha1.Storage{
		Size:  testEngineStorageSize,
		Class: &testEngineStorageClass,
	}
	expRepos := []crunchyv1beta1.PGBackRestRepo{
		{
			Name: "repo1",
			Volume: &crunchyv1beta1.RepoPVC{
				VolumeClaimSpec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteOnce,
					},
					StorageClassName: &testEngineStorageClass,
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: testEngineStorageSize,
						},
					},
				},
			},
		},
		{
			Name: "repo3",
			S3: &crunchyv1beta1.RepoS3{
				Bucket:   "bucket2",
				Region:   "region2",
				Endpoint: "endpoint2",
			},
			BackupSchedules: &crunchyv1beta1.PGBackRestBackupSchedules{
				Full: &testSchedule2,
			},
		},
		{
			Name: "repo2",
			S3: &crunchyv1beta1.RepoS3{
				Bucket:   "bucket1",
				Region:   "region1",
				Endpoint: "endpoint1",
			},
		},
	}

	repos, _, _, err := reconcilePGBackRestRepos(
		testRepos,
		testBackupSchedules,
		testBackupRequests,
		testBackupStorages,
		testBackupStoragesSecrets,
		testEngineStorage,
		&everestv1alpha1.DatabaseCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test",
				UID:  "123",
			},
		},
	)
	require.NoError(t, err)
	assert.Equal(t, expRepos, repos)
}

func TestReconcilePGBackRestReposSameStorageOneScheduleChangeSchedule(t *testing.T) {
	t.Parallel()
	testSchedule1 := "0 0 * * *"
	testSchedule2 := "0 1 * * *"
	testEngineStorageSize, _ := resource.ParseQuantity("15G")
	testEngineStorageClass := "someSC"
	testRepos := []crunchyv1beta1.PGBackRestRepo{
		{
			Name: "repo1",
			Volume: &crunchyv1beta1.RepoPVC{
				VolumeClaimSpec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteOnce,
					},
					StorageClassName: &testEngineStorageClass,
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: testEngineStorageSize,
						},
					},
				},
			},
		},
		{
			Name: "repo2",
			S3: &crunchyv1beta1.RepoS3{
				Bucket:   "bucket1",
				Region:   "region1",
				Endpoint: "endpoint1",
			},
			BackupSchedules: &crunchyv1beta1.PGBackRestBackupSchedules{
				Full: &testSchedule1,
			},
		},
	}
	testBackupSchedules := []everestv1alpha1.BackupSchedule{
		{
			Enabled:           true,
			Name:              "schedule2",
			Schedule:          testSchedule2,
			BackupStorageName: "backupStorage1",
		},
	}
	testBackupRequests := []everestv1alpha1.DatabaseClusterBackup{}
	testBackupStorages := map[string]everestv1alpha1.BackupStorageSpec{
		"backupStorage1": {
			Type:        everestv1alpha1.BackupStorageTypeS3,
			Bucket:      "bucket1",
			Region:      "region1",
			EndpointURL: "endpoint1",
		},
	}
	testBackupStoragesSecrets := map[string]*corev1.Secret{
		"backupStorage1": {
			Data: map[string][]byte{
				"AWS_ACCESS_KEY_ID":     []byte("SomeAccessKeyID"),
				"AWS_SECRET_ACCESS_KEY": []byte("SomeSecretAccessKey"),
			},
		},
	}
	testEngineStorage := everestv1alpha1.Storage{
		Size:  testEngineStorageSize,
		Class: &testEngineStorageClass,
	}
	expRepos := []crunchyv1beta1.PGBackRestRepo{
		{
			Name: "repo1",
			Volume: &crunchyv1beta1.RepoPVC{
				VolumeClaimSpec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteOnce,
					},
					StorageClassName: &testEngineStorageClass,
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: testEngineStorageSize,
						},
					},
				},
			},
		},
		{
			Name: "repo2",
			S3: &crunchyv1beta1.RepoS3{
				Bucket:   "bucket1",
				Region:   "region1",
				Endpoint: "endpoint1",
			},
			BackupSchedules: &crunchyv1beta1.PGBackRestBackupSchedules{
				Full: &testSchedule2,
			},
		},
	}

	repos, _, _, err := reconcilePGBackRestRepos(
		testRepos,
		testBackupSchedules,
		testBackupRequests,
		testBackupStorages,
		testBackupStoragesSecrets,
		testEngineStorage,
		&everestv1alpha1.DatabaseCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test",
				UID:  "123",
			},
		},
	)
	require.NoError(t, err)
	assert.Equal(t, expRepos, repos)
}

func TestReconcilePGBackRestReposSameStorageOneScheduleChangeScheduleAddSchedule(t *testing.T) {
	t.Parallel()
	testSchedule1 := "0 0 * * *"
	testSchedule2 := "0 1 * * *"
	testSchedule3 := "0 2 * * *"
	testEngineStorageSize, _ := resource.ParseQuantity("15G")
	testEngineStorageClass := "someSC"
	testRepos := []crunchyv1beta1.PGBackRestRepo{
		{
			Name: "repo1",
			Volume: &crunchyv1beta1.RepoPVC{
				VolumeClaimSpec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteOnce,
					},
					StorageClassName: &testEngineStorageClass,
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: testEngineStorageSize,
						},
					},
				},
			},
		},
		{
			Name: "repo2",
			S3: &crunchyv1beta1.RepoS3{
				Bucket:   "bucket1",
				Region:   "region1",
				Endpoint: "endpoint1",
			},
			BackupSchedules: &crunchyv1beta1.PGBackRestBackupSchedules{
				Full: &testSchedule1,
			},
		},
	}
	testBackupSchedules := []everestv1alpha1.BackupSchedule{
		{
			Enabled:           true,
			Name:              "schedule2",
			Schedule:          testSchedule2,
			BackupStorageName: "backupStorage1",
		},
		{
			Enabled:           true,
			Name:              "schedule2",
			Schedule:          testSchedule3,
			BackupStorageName: "backupStorage1",
		},
	}
	testBackupRequests := []everestv1alpha1.DatabaseClusterBackup{}
	testBackupStorages := map[string]everestv1alpha1.BackupStorageSpec{
		"backupStorage1": {
			Type:        everestv1alpha1.BackupStorageTypeS3,
			Bucket:      "bucket1",
			Region:      "region1",
			EndpointURL: "endpoint1",
		},
	}
	testBackupStoragesSecrets := map[string]*corev1.Secret{
		"backupStorage1": {
			Data: map[string][]byte{
				"AWS_ACCESS_KEY_ID":     []byte("SomeAccessKeyID"),
				"AWS_SECRET_ACCESS_KEY": []byte("SomeSecretAccessKey"),
			},
		},
	}
	testEngineStorage := everestv1alpha1.Storage{
		Size:  testEngineStorageSize,
		Class: &testEngineStorageClass,
	}
	expRepos := []crunchyv1beta1.PGBackRestRepo{
		{
			Name: "repo1",
			Volume: &crunchyv1beta1.RepoPVC{
				VolumeClaimSpec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteOnce,
					},
					StorageClassName: &testEngineStorageClass,
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: testEngineStorageSize,
						},
					},
				},
			},
		},
		{
			Name: "repo2",
			S3: &crunchyv1beta1.RepoS3{
				Bucket:   "bucket1",
				Region:   "region1",
				Endpoint: "endpoint1",
			},
			BackupSchedules: &crunchyv1beta1.PGBackRestBackupSchedules{
				Full: &testSchedule2,
			},
		},
		{
			Name: "repo3",
			S3: &crunchyv1beta1.RepoS3{
				Bucket:   "bucket1",
				Region:   "region1",
				Endpoint: "endpoint1",
			},
			BackupSchedules: &crunchyv1beta1.PGBackRestBackupSchedules{
				Full: &testSchedule3,
			},
		},
	}

	repos, _, _, err := reconcilePGBackRestRepos(
		testRepos,
		testBackupSchedules,
		testBackupRequests,
		testBackupStorages,
		testBackupStoragesSecrets,
		testEngineStorage,
		&everestv1alpha1.DatabaseCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test",
				UID:  "123",
			},
		},
	)
	require.NoError(t, err)
	assert.Equal(t, expRepos, repos)
}

func TestReconcilePGBackRestReposSameStorageOneScheduleDeleteScheduleAddRequest(t *testing.T) {
	t.Parallel()
	testSchedule := "0 0 * * *"
	testEngineStorageSize, _ := resource.ParseQuantity("15G")
	testEngineStorageClass := "someSC"
	testRepos := []crunchyv1beta1.PGBackRestRepo{
		{
			Name: "repo1",
			Volume: &crunchyv1beta1.RepoPVC{
				VolumeClaimSpec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteOnce,
					},
					StorageClassName: &testEngineStorageClass,
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: testEngineStorageSize,
						},
					},
				},
			},
		},
		{
			Name: "repo2",
			S3: &crunchyv1beta1.RepoS3{
				Bucket:   "bucket1",
				Region:   "region1",
				Endpoint: "endpoint1",
			},
			BackupSchedules: &crunchyv1beta1.PGBackRestBackupSchedules{
				Full: &testSchedule,
			},
		},
	}
	testBackupSchedules := []everestv1alpha1.BackupSchedule{}
	testBackupRequests := []everestv1alpha1.DatabaseClusterBackup{
		{
			Spec: everestv1alpha1.DatabaseClusterBackupSpec{
				BackupStorageName: "backupStorage1",
			},
		},
	}
	testBackupStorages := map[string]everestv1alpha1.BackupStorageSpec{
		"backupStorage1": {
			Type:        everestv1alpha1.BackupStorageTypeS3,
			Bucket:      "bucket1",
			Region:      "region1",
			EndpointURL: "endpoint1",
		},
	}
	testBackupStoragesSecrets := map[string]*corev1.Secret{
		"backupStorage1": {
			Data: map[string][]byte{
				"AWS_ACCESS_KEY_ID":     []byte("SomeAccessKeyID"),
				"AWS_SECRET_ACCESS_KEY": []byte("SomeSecretAccessKey"),
			},
		},
	}
	testEngineStorage := everestv1alpha1.Storage{
		Size:  testEngineStorageSize,
		Class: &testEngineStorageClass,
	}
	expRepos := []crunchyv1beta1.PGBackRestRepo{
		{
			Name: "repo1",
			Volume: &crunchyv1beta1.RepoPVC{
				VolumeClaimSpec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteOnce,
					},
					StorageClassName: &testEngineStorageClass,
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: testEngineStorageSize,
						},
					},
				},
			},
		},
		{
			Name: "repo2",
			S3: &crunchyv1beta1.RepoS3{
				Bucket:   "bucket1",
				Region:   "region1",
				Endpoint: "endpoint1",
			},
		},
	}

	repos, _, _, err := reconcilePGBackRestRepos(
		testRepos,
		testBackupSchedules,
		testBackupRequests,
		testBackupStorages,
		testBackupStoragesSecrets,
		testEngineStorage,
		&everestv1alpha1.DatabaseCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test",
				UID:  "123",
			},
		},
	)
	require.NoError(t, err)
	assert.Equal(t, expRepos, repos)
}

func TestReconcilePGBackRestReposSameStorageTwoSchedulesDelete2ndSchedule(t *testing.T) {
	t.Parallel()
	testSchedule1 := "0 0 * * *"
	testSchedule2 := "0 1 * * *"
	testEngineStorageSize, _ := resource.ParseQuantity("15G")
	testEngineStorageClass := "someSC"
	testRepos := []crunchyv1beta1.PGBackRestRepo{
		{
			Name: "repo1",
			Volume: &crunchyv1beta1.RepoPVC{
				VolumeClaimSpec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteOnce,
					},
					StorageClassName: &testEngineStorageClass,
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: testEngineStorageSize,
						},
					},
				},
			},
		},
		{
			Name: "repo2",
			S3: &crunchyv1beta1.RepoS3{
				Bucket:   "bucket1",
				Region:   "region1",
				Endpoint: "endpoint1",
			},
			BackupSchedules: &crunchyv1beta1.PGBackRestBackupSchedules{
				Full: &testSchedule1,
			},
		},
		{
			Name: "repo3",
			S3: &crunchyv1beta1.RepoS3{
				Bucket:   "bucket1",
				Region:   "region1",
				Endpoint: "endpoint1",
			},
			BackupSchedules: &crunchyv1beta1.PGBackRestBackupSchedules{
				Full: &testSchedule2,
			},
		},
	}
	testBackupSchedules := []everestv1alpha1.BackupSchedule{
		{
			Enabled:           true,
			Name:              "schedule1",
			Schedule:          testSchedule1,
			BackupStorageName: "backupStorage1",
		},
	}
	testBackupRequests := []everestv1alpha1.DatabaseClusterBackup{}
	testBackupStorages := map[string]everestv1alpha1.BackupStorageSpec{
		"backupStorage1": {
			Type:        everestv1alpha1.BackupStorageTypeS3,
			Bucket:      "bucket1",
			Region:      "region1",
			EndpointURL: "endpoint1",
		},
	}
	testBackupStoragesSecrets := map[string]*corev1.Secret{
		"backupStorage1": {
			Data: map[string][]byte{
				"AWS_ACCESS_KEY_ID":     []byte("SomeAccessKeyID"),
				"AWS_SECRET_ACCESS_KEY": []byte("SomeSecretAccessKey"),
			},
		},
	}
	testEngineStorage := everestv1alpha1.Storage{
		Size:  testEngineStorageSize,
		Class: &testEngineStorageClass,
	}
	expRepos := []crunchyv1beta1.PGBackRestRepo{
		{
			Name: "repo1",
			Volume: &crunchyv1beta1.RepoPVC{
				VolumeClaimSpec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteOnce,
					},
					StorageClassName: &testEngineStorageClass,
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: testEngineStorageSize,
						},
					},
				},
			},
		},
		{
			Name: "repo2",
			S3: &crunchyv1beta1.RepoS3{
				Bucket:   "bucket1",
				Region:   "region1",
				Endpoint: "endpoint1",
			},
			BackupSchedules: &crunchyv1beta1.PGBackRestBackupSchedules{
				Full: &testSchedule1,
			},
		},
	}

	repos, _, _, err := reconcilePGBackRestRepos(
		testRepos,
		testBackupSchedules,
		testBackupRequests,
		testBackupStorages,
		testBackupStoragesSecrets,
		testEngineStorage,
		&everestv1alpha1.DatabaseCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test",
				UID:  "123",
			},
		},
	)
	require.NoError(t, err)
	assert.Equal(t, expRepos, repos)
}

func TestReconcilePGBackRestReposSameStorageTwoSchedulesDelete1stSchedule(t *testing.T) {
	t.Parallel()
	testSchedule1 := "0 0 * * *"
	testSchedule2 := "0 1 * * *"
	testEngineStorageSize, _ := resource.ParseQuantity("15G")
	testEngineStorageClass := "someSC"
	testRepos := []crunchyv1beta1.PGBackRestRepo{
		{
			Name: "repo1",
			Volume: &crunchyv1beta1.RepoPVC{
				VolumeClaimSpec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteOnce,
					},
					StorageClassName: &testEngineStorageClass,
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: testEngineStorageSize,
						},
					},
				},
			},
		},
		{
			Name: "repo2",
			S3: &crunchyv1beta1.RepoS3{
				Bucket:   "bucket1",
				Region:   "region1",
				Endpoint: "endpoint1",
			},
			BackupSchedules: &crunchyv1beta1.PGBackRestBackupSchedules{
				Full: &testSchedule1,
			},
		},
		{
			Name: "repo3",
			S3: &crunchyv1beta1.RepoS3{
				Bucket:   "bucket1",
				Region:   "region1",
				Endpoint: "endpoint1",
			},
			BackupSchedules: &crunchyv1beta1.PGBackRestBackupSchedules{
				Full: &testSchedule2,
			},
		},
	}
	testBackupSchedules := []everestv1alpha1.BackupSchedule{
		{
			Enabled:           true,
			Name:              "schedule2",
			Schedule:          testSchedule2,
			BackupStorageName: "backupStorage1",
		},
	}
	testBackupRequests := []everestv1alpha1.DatabaseClusterBackup{}
	testBackupStorages := map[string]everestv1alpha1.BackupStorageSpec{
		"backupStorage1": {
			Type:        everestv1alpha1.BackupStorageTypeS3,
			Bucket:      "bucket1",
			Region:      "region1",
			EndpointURL: "endpoint1",
		},
	}
	testBackupStoragesSecrets := map[string]*corev1.Secret{
		"backupStorage1": {
			Data: map[string][]byte{
				"AWS_ACCESS_KEY_ID":     []byte("SomeAccessKeyID"),
				"AWS_SECRET_ACCESS_KEY": []byte("SomeSecretAccessKey"),
			},
		},
	}
	testEngineStorage := everestv1alpha1.Storage{
		Size:  testEngineStorageSize,
		Class: &testEngineStorageClass,
	}
	expRepos := []crunchyv1beta1.PGBackRestRepo{
		{
			Name: "repo1",
			Volume: &crunchyv1beta1.RepoPVC{
				VolumeClaimSpec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteOnce,
					},
					StorageClassName: &testEngineStorageClass,
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: testEngineStorageSize,
						},
					},
				},
			},
		},
		{
			Name: "repo3",
			S3: &crunchyv1beta1.RepoS3{
				Bucket:   "bucket1",
				Region:   "region1",
				Endpoint: "endpoint1",
			},
			BackupSchedules: &crunchyv1beta1.PGBackRestBackupSchedules{
				Full: &testSchedule2,
			},
		},
	}

	repos, _, _, err := reconcilePGBackRestRepos(
		testRepos,
		testBackupSchedules,
		testBackupRequests,
		testBackupStorages,
		testBackupStoragesSecrets,
		testEngineStorage,
		&everestv1alpha1.DatabaseCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test",
				UID:  "123",
			},
		},
	)
	require.NoError(t, err)
	assert.Equal(t, expRepos, repos)
}

func TestReconcilePGBackRestReposOneScheduleDeleteSchedule(t *testing.T) {
	t.Parallel()
	testSchedule1 := "0 0 * * *"
	testEngineStorageSize, _ := resource.ParseQuantity("15G")
	testEngineStorageClass := "someSC"
	testRepos := []crunchyv1beta1.PGBackRestRepo{
		{
			Name: "repo1",
			Volume: &crunchyv1beta1.RepoPVC{
				VolumeClaimSpec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteOnce,
					},
					StorageClassName: &testEngineStorageClass,
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: testEngineStorageSize,
						},
					},
				},
			},
		},
		{
			Name: "repo2",
			S3: &crunchyv1beta1.RepoS3{
				Bucket:   "bucket1",
				Region:   "region1",
				Endpoint: "endpoint1",
			},
			BackupSchedules: &crunchyv1beta1.PGBackRestBackupSchedules{
				Full: &testSchedule1,
			},
		},
	}
	testBackupSchedules := []everestv1alpha1.BackupSchedule{}
	testBackupRequests := []everestv1alpha1.DatabaseClusterBackup{}
	testBackupStorages := map[string]everestv1alpha1.BackupStorageSpec{}
	testBackupStoragesSecrets := map[string]*corev1.Secret{}
	testEngineStorage := everestv1alpha1.Storage{
		Size:  testEngineStorageSize,
		Class: &testEngineStorageClass,
	}
	expRepos := []crunchyv1beta1.PGBackRestRepo{
		{
			Name: "repo1",
			Volume: &crunchyv1beta1.RepoPVC{
				VolumeClaimSpec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteOnce,
					},
					StorageClassName: &testEngineStorageClass,
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: testEngineStorageSize,
						},
					},
				},
			},
		},
	}

	repos, _, _, err := reconcilePGBackRestRepos(
		testRepos,
		testBackupSchedules,
		testBackupRequests,
		testBackupStorages,
		testBackupStoragesSecrets,
		testEngineStorage,
		&everestv1alpha1.DatabaseCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test",
				UID:  "123",
			},
		},
	)
	require.NoError(t, err)
	assert.Equal(t, expRepos, repos)
}

func TestReconcilePGBackRestReposOneRequestDeleteRequest(t *testing.T) {
	t.Parallel()
	testEngineStorageSize, _ := resource.ParseQuantity("15G")
	testEngineStorageClass := "someSC"
	testRepos := []crunchyv1beta1.PGBackRestRepo{
		{
			Name: "repo1",
			Volume: &crunchyv1beta1.RepoPVC{
				VolumeClaimSpec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteOnce,
					},
					StorageClassName: &testEngineStorageClass,
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: testEngineStorageSize,
						},
					},
				},
			},
		},
		{
			Name: "repo2",
			S3: &crunchyv1beta1.RepoS3{
				Bucket:   "bucket1",
				Region:   "region1",
				Endpoint: "endpoint1",
			},
		},
	}
	testBackupSchedules := []everestv1alpha1.BackupSchedule{}
	testBackupRequests := []everestv1alpha1.DatabaseClusterBackup{}
	testBackupStorages := map[string]everestv1alpha1.BackupStorageSpec{}
	testBackupStoragesSecrets := map[string]*corev1.Secret{}
	testEngineStorage := everestv1alpha1.Storage{
		Size:  testEngineStorageSize,
		Class: &testEngineStorageClass,
	}
	expRepos := []crunchyv1beta1.PGBackRestRepo{
		{
			Name: "repo1",
			Volume: &crunchyv1beta1.RepoPVC{
				VolumeClaimSpec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteOnce,
					},
					StorageClassName: &testEngineStorageClass,
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: testEngineStorageSize,
						},
					},
				},
			},
		},
	}

	repos, _, _, err := reconcilePGBackRestRepos(
		testRepos,
		testBackupSchedules,
		testBackupRequests,
		testBackupStorages,
		testBackupStoragesSecrets,
		testEngineStorage,
		&everestv1alpha1.DatabaseCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test",
				UID:  "123",
			},
		},
	)
	require.NoError(t, err)
	assert.Equal(t, expRepos, repos)
}

func TestReconcilePGBackRestReposSameStorageThreeSchedulesAddSchedule(t *testing.T) {
	t.Parallel()
	testSchedule1 := "0 0 * * *"
	testSchedule2 := "0 1 * * *"
	testSchedule3 := "0 2 * * *"
	testSchedule4 := "0 3 * * *"
	testEngineStorageSize, _ := resource.ParseQuantity("15G")
	testEngineStorageClass := "someSC"
	testRepos := []crunchyv1beta1.PGBackRestRepo{
		{
			Name: "repo1",
			Volume: &crunchyv1beta1.RepoPVC{
				VolumeClaimSpec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteOnce,
					},
					StorageClassName: &testEngineStorageClass,
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: testEngineStorageSize,
						},
					},
				},
			},
		},
		{
			Name: "repo2",
			S3: &crunchyv1beta1.RepoS3{
				Bucket:   "bucket1",
				Region:   "region1",
				Endpoint: "endpoint1",
			},
			BackupSchedules: &crunchyv1beta1.PGBackRestBackupSchedules{
				Full: &testSchedule1,
			},
		},
		{
			Name: "repo3",
			S3: &crunchyv1beta1.RepoS3{
				Bucket:   "bucket1",
				Region:   "region1",
				Endpoint: "endpoint1",
			},
			BackupSchedules: &crunchyv1beta1.PGBackRestBackupSchedules{
				Full: &testSchedule2,
			},
		},
		{
			Name: "repo4",
			S3: &crunchyv1beta1.RepoS3{
				Bucket:   "bucket1",
				Region:   "region1",
				Endpoint: "endpoint1",
			},
			BackupSchedules: &crunchyv1beta1.PGBackRestBackupSchedules{
				Full: &testSchedule3,
			},
		},
	}
	testBackupSchedules := []everestv1alpha1.BackupSchedule{
		{
			Enabled:           true,
			Name:              "schedule1",
			Schedule:          testSchedule1,
			BackupStorageName: "backupStorage1",
		},
		{
			Enabled:           true,
			Name:              "schedule2",
			Schedule:          testSchedule2,
			BackupStorageName: "backupStorage1",
		},
		{
			Enabled:           true,
			Name:              "schedule3",
			Schedule:          testSchedule3,
			BackupStorageName: "backupStorage1",
		},
		{
			Enabled:           true,
			Name:              "schedule4",
			Schedule:          testSchedule4,
			BackupStorageName: "backupStorage1",
		},
	}
	testBackupRequests := []everestv1alpha1.DatabaseClusterBackup{}
	testBackupStorages := map[string]everestv1alpha1.BackupStorageSpec{
		"backupStorage1": {
			Type:        everestv1alpha1.BackupStorageTypeS3,
			Bucket:      "bucket1",
			Region:      "region1",
			EndpointURL: "endpoint1",
		},
	}
	testBackupStoragesSecrets := map[string]*corev1.Secret{
		"backupStorage1": {
			Data: map[string][]byte{
				"AWS_ACCESS_KEY_ID":     []byte("SomeAccessKeyID"),
				"AWS_SECRET_ACCESS_KEY": []byte("SomeSecretAccessKey"),
			},
		},
	}
	testEngineStorage := everestv1alpha1.Storage{
		Size:  testEngineStorageSize,
		Class: &testEngineStorageClass,
	}
	expRepos := []crunchyv1beta1.PGBackRestRepo{}

	repos, _, _, err := reconcilePGBackRestRepos(
		testRepos,
		testBackupSchedules,
		testBackupRequests,
		testBackupStorages,
		testBackupStoragesSecrets,
		testEngineStorage,
		&everestv1alpha1.DatabaseCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test",
				UID:  "123",
			},
		},
	)
	require.Error(t, err)
	assert.Equal(t, expRepos, repos)
}

func TestReconcilePGBackRestReposDifferentStorageThreeRequestsAddRequest(t *testing.T) {
	t.Parallel()
	testEngineStorageSize, _ := resource.ParseQuantity("15G")
	testEngineStorageClass := "someSC"
	testRepos := []crunchyv1beta1.PGBackRestRepo{
		{
			Name: "repo1",
			Volume: &crunchyv1beta1.RepoPVC{
				VolumeClaimSpec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteOnce,
					},
					StorageClassName: &testEngineStorageClass,
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: testEngineStorageSize,
						},
					},
				},
			},
		},
		{
			Name: "repo2",
			S3: &crunchyv1beta1.RepoS3{
				Bucket:   "bucket1",
				Region:   "region1",
				Endpoint: "endpoint1",
			},
		},
		{
			Name: "repo3",
			S3: &crunchyv1beta1.RepoS3{
				Bucket:   "bucket2",
				Region:   "region2",
				Endpoint: "endpoint2",
			},
		},
		{
			Name: "repo4",
			S3: &crunchyv1beta1.RepoS3{
				Bucket:   "bucket3",
				Region:   "region3",
				Endpoint: "endpoint3",
			},
		},
	}
	testBackupSchedules := []everestv1alpha1.BackupSchedule{}
	testBackupRequests := []everestv1alpha1.DatabaseClusterBackup{
		{
			Spec: everestv1alpha1.DatabaseClusterBackupSpec{
				BackupStorageName: "backupStorage1",
			},
		},
		{
			Spec: everestv1alpha1.DatabaseClusterBackupSpec{
				BackupStorageName: "backupStorage2",
			},
		},
		{
			Spec: everestv1alpha1.DatabaseClusterBackupSpec{
				BackupStorageName: "backupStorage3",
			},
		},
		{
			Spec: everestv1alpha1.DatabaseClusterBackupSpec{
				BackupStorageName: "backupStorage4",
			},
		},
	}
	testBackupStorages := map[string]everestv1alpha1.BackupStorageSpec{
		"backupStorage1": {
			Type:        everestv1alpha1.BackupStorageTypeS3,
			Bucket:      "bucket1",
			Region:      "region1",
			EndpointURL: "endpoint1",
		},
		"backupStorage2": {
			Type:        everestv1alpha1.BackupStorageTypeS3,
			Bucket:      "bucket2",
			Region:      "region2",
			EndpointURL: "endpoint2",
		},
		"backupStorage3": {
			Type:        everestv1alpha1.BackupStorageTypeS3,
			Bucket:      "bucket3",
			Region:      "region3",
			EndpointURL: "endpoint3",
		},
		"backupStorage4": {
			Type:        everestv1alpha1.BackupStorageTypeS3,
			Bucket:      "bucket4",
			Region:      "region4",
			EndpointURL: "endpoint4",
		},
	}
	testBackupStoragesSecrets := map[string]*corev1.Secret{
		"backupStorage1": {
			Data: map[string][]byte{
				"AWS_ACCESS_KEY_ID":     []byte("SomeAccessKeyID"),
				"AWS_SECRET_ACCESS_KEY": []byte("SomeSecretAccessKey"),
			},
		},
		"backupStorage2": {
			Data: map[string][]byte{
				"AWS_ACCESS_KEY_ID":     []byte("SomeAccessKeyID"),
				"AWS_SECRET_ACCESS_KEY": []byte("SomeSecretAccessKey"),
			},
		},
		"backupStorage3": {
			Data: map[string][]byte{
				"AWS_ACCESS_KEY_ID":     []byte("SomeAccessKeyID"),
				"AWS_SECRET_ACCESS_KEY": []byte("SomeSecretAccessKey"),
			},
		},
		"backupStorage4": {
			Data: map[string][]byte{
				"AWS_ACCESS_KEY_ID":     []byte("SomeAccessKeyID"),
				"AWS_SECRET_ACCESS_KEY": []byte("SomeSecretAccessKey"),
			},
		},
	}
	testEngineStorage := everestv1alpha1.Storage{
		Size:  testEngineStorageSize,
		Class: &testEngineStorageClass,
	}
	expRepos := []crunchyv1beta1.PGBackRestRepo{}

	repos, _, _, err := reconcilePGBackRestRepos(
		testRepos,
		testBackupSchedules,
		testBackupRequests,
		testBackupStorages,
		testBackupStoragesSecrets,
		testEngineStorage,
		&everestv1alpha1.DatabaseCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test",
				UID:  "123",
			},
		},
	)
	require.Error(t, err)
	assert.Equal(t, expRepos, repos)
}

func TestReconcilePGBackRestReposSameStorageThreeSchedulesAddRequest(t *testing.T) {
	t.Parallel()
	testSchedule1 := "0 0 * * *"
	testSchedule2 := "0 1 * * *"
	testSchedule3 := "0 2 * * *"
	testEngineStorageSize, _ := resource.ParseQuantity("15G")
	testEngineStorageClass := "someSC"
	testRepos := []crunchyv1beta1.PGBackRestRepo{
		{
			Name: "repo1",
			Volume: &crunchyv1beta1.RepoPVC{
				VolumeClaimSpec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteOnce,
					},
					StorageClassName: &testEngineStorageClass,
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: testEngineStorageSize,
						},
					},
				},
			},
		},
		{
			Name: "repo2",
			S3: &crunchyv1beta1.RepoS3{
				Bucket:   "bucket1",
				Region:   "region1",
				Endpoint: "endpoint1",
			},
			BackupSchedules: &crunchyv1beta1.PGBackRestBackupSchedules{
				Full: &testSchedule1,
			},
		},
		{
			Name: "repo3",
			S3: &crunchyv1beta1.RepoS3{
				Bucket:   "bucket1",
				Region:   "region1",
				Endpoint: "endpoint1",
			},
			BackupSchedules: &crunchyv1beta1.PGBackRestBackupSchedules{
				Full: &testSchedule2,
			},
		},
		{
			Name: "repo4",
			S3: &crunchyv1beta1.RepoS3{
				Bucket:   "bucket1",
				Region:   "region1",
				Endpoint: "endpoint1",
			},
			BackupSchedules: &crunchyv1beta1.PGBackRestBackupSchedules{
				Full: &testSchedule3,
			},
		},
	}
	testBackupSchedules := []everestv1alpha1.BackupSchedule{
		{
			Enabled:           true,
			Name:              "schedule1",
			Schedule:          testSchedule1,
			BackupStorageName: "backupStorage1",
		},
		{
			Enabled:           true,
			Name:              "schedule2",
			Schedule:          testSchedule2,
			BackupStorageName: "backupStorage1",
		},
		{
			Enabled:           true,
			Name:              "schedule3",
			Schedule:          testSchedule3,
			BackupStorageName: "backupStorage1",
		},
	}
	testBackupRequests := []everestv1alpha1.DatabaseClusterBackup{
		{
			Spec: everestv1alpha1.DatabaseClusterBackupSpec{
				BackupStorageName: "backupStorage1",
			},
		},
	}
	testBackupStorages := map[string]everestv1alpha1.BackupStorageSpec{
		"backupStorage1": {
			Type:        everestv1alpha1.BackupStorageTypeS3,
			Bucket:      "bucket1",
			Region:      "region1",
			EndpointURL: "endpoint1",
		},
	}
	testBackupStoragesSecrets := map[string]*corev1.Secret{
		"backupStorage1": {
			Data: map[string][]byte{
				"AWS_ACCESS_KEY_ID":     []byte("SomeAccessKeyID"),
				"AWS_SECRET_ACCESS_KEY": []byte("SomeSecretAccessKey"),
			},
		},
	}
	testEngineStorage := everestv1alpha1.Storage{
		Size:  testEngineStorageSize,
		Class: &testEngineStorageClass,
	}
	expRepos := []crunchyv1beta1.PGBackRestRepo{
		{
			Name: "repo1",
			Volume: &crunchyv1beta1.RepoPVC{
				VolumeClaimSpec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteOnce,
					},
					StorageClassName: &testEngineStorageClass,
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: testEngineStorageSize,
						},
					},
				},
			},
		},
		{
			Name: "repo2",
			S3: &crunchyv1beta1.RepoS3{
				Bucket:   "bucket1",
				Region:   "region1",
				Endpoint: "endpoint1",
			},
			BackupSchedules: &crunchyv1beta1.PGBackRestBackupSchedules{
				Full: &testSchedule1,
			},
		},
		{
			Name: "repo3",
			S3: &crunchyv1beta1.RepoS3{
				Bucket:   "bucket1",
				Region:   "region1",
				Endpoint: "endpoint1",
			},
			BackupSchedules: &crunchyv1beta1.PGBackRestBackupSchedules{
				Full: &testSchedule2,
			},
		},
		{
			Name: "repo4",
			S3: &crunchyv1beta1.RepoS3{
				Bucket:   "bucket1",
				Region:   "region1",
				Endpoint: "endpoint1",
			},
			BackupSchedules: &crunchyv1beta1.PGBackRestBackupSchedules{
				Full: &testSchedule3,
			},
		},
	}

	repos, _, _, err := reconcilePGBackRestRepos(
		testRepos,
		testBackupSchedules,
		testBackupRequests,
		testBackupStorages,
		testBackupStoragesSecrets,
		testEngineStorage,
		&everestv1alpha1.DatabaseCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test",
				UID:  "123",
			},
		},
	)
	require.NoError(t, err)
	assert.Equal(t, expRepos, repos)
}

func TestReconcilePGBackRestReposUnknownStorageRequest(t *testing.T) {
	t.Parallel()
	testRepos := []crunchyv1beta1.PGBackRestRepo{}
	testBackupSchedules := []everestv1alpha1.BackupSchedule{}
	testBackupRequests := []everestv1alpha1.DatabaseClusterBackup{
		{
			Spec: everestv1alpha1.DatabaseClusterBackupSpec{
				BackupStorageName: "backupStorage1",
			},
		},
	}
	testBackupStorages := map[string]everestv1alpha1.BackupStorageSpec{}
	testBackupStoragesSecrets := map[string]*corev1.Secret{}
	testEngineStorageSize, _ := resource.ParseQuantity("15G")
	testEngineStorageClass := "someSC"
	testEngineStorage := everestv1alpha1.Storage{
		Size:  testEngineStorageSize,
		Class: &testEngineStorageClass,
	}
	expRepos := []crunchyv1beta1.PGBackRestRepo{}

	repos, _, _, err := reconcilePGBackRestRepos(
		testRepos,
		testBackupSchedules,
		testBackupRequests,
		testBackupStorages,
		testBackupStoragesSecrets,
		testEngineStorage,
		&everestv1alpha1.DatabaseCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test",
				UID:  "123",
			},
		},
	)
	require.Error(t, err)
	assert.Equal(t, expRepos, repos)
}

func TestReconcilePGBackRestReposUnknownStorageSchedule(t *testing.T) {
	t.Parallel()
	testSchedule := "0 0 * * *"
	testRepos := []crunchyv1beta1.PGBackRestRepo{}
	testBackupSchedules := []everestv1alpha1.BackupSchedule{
		{
			Enabled:           true,
			Name:              "schedule1",
			Schedule:          testSchedule,
			BackupStorageName: "backupStorage1",
		},
	}
	testBackupRequests := []everestv1alpha1.DatabaseClusterBackup{}
	testBackupStorages := map[string]everestv1alpha1.BackupStorageSpec{}
	testBackupStoragesSecrets := map[string]*corev1.Secret{}
	testEngineStorageSize, _ := resource.ParseQuantity("15G")
	testEngineStorageClass := "someSC"
	testEngineStorage := everestv1alpha1.Storage{
		Size:  testEngineStorageSize,
		Class: &testEngineStorageClass,
	}
	expRepos := []crunchyv1beta1.PGBackRestRepo{}

	repos, _, _, err := reconcilePGBackRestRepos(
		testRepos,
		testBackupSchedules,
		testBackupRequests,
		testBackupStorages,
		testBackupStoragesSecrets,
		testEngineStorage,
		&everestv1alpha1.DatabaseCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test",
				UID:  "123",
			},
		},
	)
	require.Error(t, err)
	assert.Equal(t, expRepos, repos)
}

func TestReconcilePGBackRestReposEmpty(t *testing.T) {
	t.Parallel()
	testRepos := []crunchyv1beta1.PGBackRestRepo{}
	testBackupSchedules := []everestv1alpha1.BackupSchedule{}
	testBackupRequests := []everestv1alpha1.DatabaseClusterBackup{}
	testBackupStorages := map[string]everestv1alpha1.BackupStorageSpec{}
	testBackupStoragesSecrets := map[string]*corev1.Secret{}
	testEngineStorageSize, _ := resource.ParseQuantity("15G")
	testEngineStorageClass := "someSC"
	testEngineStorage := everestv1alpha1.Storage{
		Size:  testEngineStorageSize,
		Class: &testEngineStorageClass,
	}
	expRepos := []crunchyv1beta1.PGBackRestRepo{
		{
			Name: "repo1",
			Volume: &crunchyv1beta1.RepoPVC{
				VolumeClaimSpec: corev1.PersistentVolumeClaimSpec{
					AccessModes: []corev1.PersistentVolumeAccessMode{
						corev1.ReadWriteOnce,
					},
					StorageClassName: &testEngineStorageClass,
					Resources: corev1.VolumeResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceStorage: testEngineStorageSize,
						},
					},
				},
			},
		},
	}

	repos, _, _, err := reconcilePGBackRestRepos(
		testRepos,
		testBackupSchedules,
		testBackupRequests,
		testBackupStorages,
		testBackupStoragesSecrets,
		testEngineStorage,
		&everestv1alpha1.DatabaseCluster{
			ObjectMeta: metav1.ObjectMeta{
				Name: "test",
				UID:  "123",
			},
		},
	)
	require.NoError(t, err)
	assert.Equal(t, expRepos, repos)
}

func Test_globalDatasourceDestination(t *testing.T) {
	t.Parallel()

	t.Run("empty dest", func(t *testing.T) {
		t.Parallel()

		db := &everestv1alpha1.DatabaseCluster{}
		db.Name = "db-name"
		db.UID = "db-uid"

		bs := &everestv1alpha1.BackupStorage{}

		dest := globalDatasourceDestination("", db, bs)
		assert.Equal(t, "/"+backupStoragePrefix(db), dest)
	})

	t.Run("not-empty dest s3", func(t *testing.T) {
		t.Parallel()

		db := &everestv1alpha1.DatabaseCluster{}
		db.Name = "db-name"
		db.UID = "db-uid"

		bs := &everestv1alpha1.BackupStorage{
			Spec: everestv1alpha1.BackupStorageSpec{
				Type:   everestv1alpha1.BackupStorageTypeS3,
				Bucket: "some/bucket/here",
			},
		}

		dest := globalDatasourceDestination("s3://some/bucket/here/db-name/db-uid/some/folders/later", db, bs)
		assert.Equal(t, "/db-name/db-uid", dest)
	})

	t.Run("not-empty dest azure", func(t *testing.T) {
		t.Parallel()

		db := &everestv1alpha1.DatabaseCluster{}
		db.Name = "db-name"
		db.UID = "db-uid"

		bs := &everestv1alpha1.BackupStorage{
			Spec: everestv1alpha1.BackupStorageSpec{
				Type:   everestv1alpha1.BackupStorageTypeAzure,
				Bucket: "some/bucket/here",
			},
		}

		dest := globalDatasourceDestination("azure://some/bucket/here/db-name/db-uid/some/folders/later", db, bs)
		assert.Equal(t, "/db-name/db-uid", dest)
	})
}
