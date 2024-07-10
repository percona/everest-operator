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

// Package common contains common utilities for the everest-operator.
package common

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"reflect"
	"strings"

	"github.com/AlekSi/pointer"
	crunchyv1beta1 "github.com/percona/percona-postgresql-operator/pkg/apis/postgres-operator.crunchydata.com/v1beta1"
	psmdbv1 "github.com/percona/percona-server-mongodb-operator/pkg/apis/psmdb/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	everestv1alpha1 "github.com/percona/everest-operator/api/v1alpha1"
	"github.com/percona/everest-operator/controllers/version"
)

// PITRBucketName returns the name of the bucket for the point-in-time recovery backups.
func PITRBucketName(db *everestv1alpha1.DatabaseCluster, bucket string) string {
	return fmt.Sprintf("%s/%s/%s", bucket, BackupStoragePrefix(db), "pitr")
}

// PITRStorageName returns the name of the storage for the point-in-time recovery backups.
func PITRStorageName(storageName string) string {
	return storageName + "-pitr"
}

// BackupStoragePrefix returns the prefix for the backup storage.
func BackupStoragePrefix(db *everestv1alpha1.DatabaseCluster) string {
	return fmt.Sprintf("%s/%s", db.Name, db.UID)
}

// GetOperatorVersion returns the version of the operator running in the cluster
// for the specified deployment name and namespace.
//
// TODO: Read the operator version from the DatabaseEngine status rather than fetching the Deployment,
// since DatabaseEngines are cached in the controller's client.
//
//nolint:godox
func GetOperatorVersion(
	ctx context.Context,
	c client.Client,
	name types.NamespacedName,
) (*version.Version, error) {
	unstructuredResource := &unstructured.Unstructured{}
	unstructuredResource.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "apps",
		Kind:    "Deployment",
		Version: "v1",
	})
	deployment := &appsv1.Deployment{}
	if err := c.Get(ctx, name, unstructuredResource); err != nil {
		return nil, err
	}
	err := runtime.DefaultUnstructuredConverter.
		FromUnstructured(unstructuredResource.Object, deployment)
	if err != nil {
		return nil, err
	}
	v := strings.Split(deployment.Spec.Template.Spec.Containers[0].Image, ":")[1]
	return version.NewVersion(v)
}

// GetClusterType returns the type of the cluster on which this operator is running.
func GetClusterType(ctx context.Context, c client.Client) (ClusterType, error) {
	clusterType := ClusterTypeMinikube
	unstructuredResource := &unstructured.Unstructured{}
	unstructuredResource.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "storage.k8s.io",
		Kind:    "StorageClass",
		Version: "v1",
	})
	storageList := &storagev1.StorageClassList{}

	err := c.List(ctx, unstructuredResource)
	if err != nil {
		return clusterType, err
	}
	err = runtime.DefaultUnstructuredConverter.
		FromUnstructured(unstructuredResource.Object, storageList)
	if err != nil {
		return clusterType, err
	}
	for _, storage := range storageList.Items {
		if strings.Contains(storage.Provisioner, "aws") {
			clusterType = ClusterTypeEKS
		}
	}
	return clusterType, nil
}

func mergeMap(dst map[string]interface{}, src map[string]interface{}) error {
	return mergeMapInternal(dst, src, "")
}

func mergeMapInternal(dst map[string]interface{}, src map[string]interface{}, parent string) error {
	for k, v := range src {
		if dst[k] != nil && reflect.TypeOf(dst[k]) != reflect.TypeOf(v) {
			return fmt.Errorf("type mismatch for %s.%s, %T != %T", parent, k, dst[k], v)
		}
		switch v.(type) { //nolint:gocritic
		case map[string]interface{}:
			switch dst[k].(type) { //nolint:gocritic
			case nil:
				dst[k] = v
			case map[string]interface{}: //nolint:forcetypeassert
				err := mergeMapInternal(dst[k].(map[string]interface{}),
					v.(map[string]interface{}), fmt.Sprintf("%s.%s", parent, k))
				if err != nil {
					return err
				}
			default:
				return fmt.Errorf("type mismatch for %s.%s, %T != %T", parent, k, dst[k], v)
			}
		default:
			dst[k] = v
		}
	}
	return nil
}

// GetDatabaseEngine gets the DatabaseEngine object with the specified name and namespace.
func GetDatabaseEngine(ctx context.Context, c client.Client, name, namespace string) (*everestv1alpha1.DatabaseEngine, error) {
	engine := &everestv1alpha1.DatabaseEngine{}
	err := c.Get(ctx,
		types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		}, engine)
	if err != nil {
		return nil, fmt.Errorf("failed to get database engine %s", name)
	}
	return engine, nil
}

// GetSecretFromMonitoringConfig gets the secret data from the MonitoringConfig.
func GetSecretFromMonitoringConfig(
	ctx context.Context,
	c client.Client,
	monitoring *everestv1alpha1.MonitoringConfig,
	ns string,
) (string, error) {
	var secret *corev1.Secret
	secretData := ""

	if monitoring.Spec.CredentialsSecretName != "" {
		secret = &corev1.Secret{}
		err := c.Get(ctx, types.NamespacedName{
			Name:      monitoring.Spec.CredentialsSecretName,
			Namespace: ns,
		}, secret)
		if err != nil {
			return "", err
		}

		if key, ok := secret.Data[everestv1alpha1.MonitoringConfigCredentialsSecretAPIKeyKey]; ok {
			secretData = string(key)
		}
	}

	return secretData, nil
}

// UpdateSecretData updates the data of a secret.
// It only changes the values of the keys specified in the data map.
// All other keys are left untouched, so it's not possible to delete a key.
func UpdateSecretData(
	ctx context.Context,
	c client.Client,
	database *everestv1alpha1.DatabaseCluster,
	secretName string,
	data map[string][]byte,
) error {
	secret := &corev1.Secret{}
	err := c.Get(ctx, types.NamespacedName{
		Name:      secretName,
		Namespace: database.Namespace,
	}, secret)
	if err != nil {
		return err
	}

	var needsUpdate bool
	for k, v := range data {
		oldValue, ok := secret.Data[k]
		if !ok || !bytes.Equal(oldValue, v) {
			secret.Data[k] = v
			needsUpdate = true
		}
	}
	if !needsUpdate {
		return nil
	}

	err = controllerutil.SetControllerReference(database, secret, c.Scheme())
	if err != nil {
		return err
	}
	return c.Update(ctx, secret)
}

// CreateOrUpdateSecretData creates or updates the data of a secret.
// When updating, it only changes the values of the keys specified in the data
// map.
// All other keys are left untouched, so it's not possible to delete a key.
func CreateOrUpdateSecretData(
	ctx context.Context,
	c client.Client,
	database *everestv1alpha1.DatabaseCluster,
	secretName string,
	data map[string][]byte,
) error {
	err := UpdateSecretData(ctx, c, database, secretName, data)
	if err == nil {
		return nil
	}

	if !k8serrors.IsNotFound(err) {
		return err
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: database.Namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: data,
	}
	err = controllerutil.SetControllerReference(database, secret, c.Scheme())
	if err != nil {
		return err
	}
	// If the secret does not exist, create it
	return c.Create(ctx, secret)
}

// ReconcileBackupStorageSecret reconciles the backup storage secret.
func ReconcileBackupStorageSecret(
	ctx context.Context,
	c client.Client,
	systemNs string,
	backupStorage *everestv1alpha1.BackupStorage,
	database *everestv1alpha1.DatabaseCluster,
) error {
	secret := &corev1.Secret{}
	err := c.Get(ctx, types.NamespacedName{Name: backupStorage.Spec.CredentialsSecretName, Namespace: database.Namespace}, secret)
	if err != nil && !k8serrors.IsNotFound(err) {
		return err
	}
	if secret.Name != "" {
		return nil
	}

	err = c.Get(ctx, types.NamespacedName{Name: backupStorage.Spec.CredentialsSecretName, Namespace: systemNs}, secret)
	if err != nil {
		return err
	}
	secret.ObjectMeta = metav1.ObjectMeta{
		Namespace: database.Namespace,
		Name:      backupStorage.Spec.CredentialsSecretName,
		Labels: map[string]string{
			LabelBackupStorageName: backupStorage.Name,
		},
	}
	if !backupStorage.IsNamespaceAllowed(database.Namespace) {
		return fmt.Errorf("%s namespace is not allowed to use for %s backup storage", database.Namespace, backupStorage.Name)
	}
	return c.Create(ctx, secret)
}

// ReconcileDBRestoreFromDataSource reconciles the DatabaseClusterRestore object
// based on the DataSource field of the DatabaseCluster.
func ReconcileDBRestoreFromDataSource(
	ctx context.Context,
	c client.Client,
	database *everestv1alpha1.DatabaseCluster,
) error {
	dbr := &everestv1alpha1.DatabaseClusterRestore{}
	err := c.Get(ctx, types.NamespacedName{
		Namespace: database.Namespace,
		Name:      database.Name,
	}, dbr)
	if err != nil && !k8serrors.IsNotFound(err) {
		return err
	}
	if dbr.IsComplete() {
		database.Spec.DataSource = nil
		return nil
	}

	if (database.Spec.DataSource.DBClusterBackupName == "" && database.Spec.DataSource.BackupSource == nil) ||
		(database.Spec.DataSource.DBClusterBackupName != "" && database.Spec.DataSource.BackupSource != nil) {
		return errors.New("either DBClusterBackupName or BackupSource must be specified in the DataSource field")
	}

	dbRestore := &everestv1alpha1.DatabaseClusterRestore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      database.Name,
			Namespace: database.Namespace,
		},
	}
	if err := controllerutil.SetControllerReference(database, dbRestore, c.Scheme()); err != nil {
		return err
	}
	_, err = controllerutil.CreateOrUpdate(ctx, c, dbRestore, func() error {
		dbRestore.Spec.DBClusterName = database.Name
		dbRestore.Spec.DataSource = getRestoreDataSource(ctx, c, database)
		return nil
	})

	return err
}

func getRestoreDataSource(
	ctx context.Context,
	c client.Client,
	database *everestv1alpha1.DatabaseCluster,
) everestv1alpha1.DataSource {
	if database.Spec.Engine.Type != everestv1alpha1.DatabaseEnginePSMDB || database.Spec.DataSource.DBClusterBackupName == "" {
		return *database.Spec.DataSource
	}

	// Use the full backup source instead of just the backupName to be able to
	// figure out the source dbc's prefix in the storage
	backupName := database.Spec.DataSource.DBClusterBackupName

	backup := &psmdbv1.PerconaServerMongoDBBackup{}
	err := c.Get(ctx, types.NamespacedName{Name: backupName, Namespace: database.Namespace}, backup)
	if err != nil {
		return everestv1alpha1.DataSource{}
	}

	return everestv1alpha1.DataSource{
		BackupSource: &everestv1alpha1.BackupSource{
			Path:              backup.Status.Destination,
			BackupStorageName: backup.Spec.StorageName,
		},
		PITR: database.Spec.DataSource.PITR,
	}
}

// ValidatePitrRestoreSpec validates the PITR restore spec.
func ValidatePitrRestoreSpec(dataSource everestv1alpha1.DataSource) error {
	if dataSource.PITR == nil {
		return nil
	}

	// use 'date' as default
	if dataSource.PITR.Type == "" {
		dataSource.PITR.Type = everestv1alpha1.PITRTypeDate
	}

	switch dataSource.PITR.Type {
	case everestv1alpha1.PITRTypeDate:
		if dataSource.PITR.Date == nil {
			return ErrPitrEmptyDate
		}
	case everestv1alpha1.PITRTypeLatest:
		//nolint:godox
		// TODO: figure out why "latest" doesn't work currently for Everest
		return ErrPitrTypeLatest
	default:
		return ErrPitrTypeIsNotSupported
	}
	return nil
}

// CreateOrUpdate creates or updates a resource.
// With patchSecretData the new secret Data is applied on top of the original secret's Data.
func CreateOrUpdate(
	ctx context.Context,
	c client.Client,
	obj client.Object,
	patchSecretData bool,
) error {
	hash, err := getObjectHash(obj)
	if err != nil {
		return errors.Join(err, errors.New("calculate object hash"))
	}

	val := reflect.ValueOf(obj)
	if val.Kind() == reflect.Ptr {
		val = reflect.Indirect(val)
	}
	oldObject, ok := reflect.New(val.Type()).Interface().(client.Object)
	if !ok {
		return errors.New("failed type conversion")
	}

	if obj.GetName() == "" && obj.GetGenerateName() != "" {
		return c.Create(ctx, obj)
	}

	err = c.Get(ctx, types.NamespacedName{
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
	}, oldObject)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return c.Create(ctx, obj)
		}
		return errors.Join(err, errors.New("get object"))
	}

	oldHash, err := getObjectHash(oldObject)
	if err != nil {
		return errors.Join(err, errors.New("calculate old object hash"))
	}

	if oldHash != hash || !isObjectMetaEqual(obj, oldObject) {
		return updateObject(ctx, c, obj, oldObject, patchSecretData)
	}

	return nil
}

func getObjectHash(obj runtime.Object) (string, error) {
	var dataToMarshall interface{}
	switch object := obj.(type) {
	case *appsv1.StatefulSet:
		dataToMarshall = object.Spec
	case *appsv1.Deployment:
		dataToMarshall = object.Spec
	case *corev1.Service:
		dataToMarshall = object.Spec
	default:
		dataToMarshall = obj
	}
	data, err := json.Marshal(dataToMarshall)
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(data), nil
}

func isObjectMetaEqual(oldObj, newObj metav1.Object) bool {
	return reflect.DeepEqual(oldObj.GetAnnotations(), newObj.GetAnnotations()) &&
		reflect.DeepEqual(oldObj.GetLabels(), newObj.GetLabels())
}

func updateObject(
	ctx context.Context,
	c client.Client,
	obj, oldObject client.Object,
	patchSecretData bool,
) error {
	obj.SetResourceVersion(oldObject.GetResourceVersion())
	switch object := obj.(type) {
	case *corev1.Service:
		oldObjectService, ok := oldObject.(*corev1.Service)
		if !ok {
			return errors.New("failed type conversion to service")
		}
		object.Spec.ClusterIP = oldObjectService.Spec.ClusterIP
		if object.Spec.Type == corev1.ServiceTypeLoadBalancer {
			object.Spec.HealthCheckNodePort = oldObjectService.Spec.HealthCheckNodePort
		}
	case *corev1.Secret:
		if patchSecretData {
			s, ok := oldObject.(*corev1.Secret)
			if !ok {
				return errors.New("failed type conversion to secret")
			}
			for k, v := range object.Data {
				s.Data[k] = v
			}
			object.Data = s.Data
		}
	default:
	}

	return c.Update(ctx, obj)
}

// GetBackupStorage returns a BackupStorage object
// with the specified name and namespace.
func GetBackupStorage(
	ctx context.Context,
	c client.Client,
	name, namespace string,
) (*everestv1alpha1.BackupStorage, error) {
	backupStorage := &everestv1alpha1.BackupStorage{}
	key := types.NamespacedName{Name: name, Namespace: namespace}
	if err := c.Get(ctx, key, backupStorage); err != nil {
		return nil,
			fmt.Errorf("failed to get backup storage '%s': %w", name, err)
	}
	return backupStorage, nil
}

// ListDatabaseClusterBackups returns a list of DatabaseClusterBackup objects
// for the DatabaseCluster with the specified name and namespace.
func ListDatabaseClusterBackups(
	ctx context.Context,
	c client.Client,
	dbName, dbNs string,
) (*everestv1alpha1.DatabaseClusterBackupList, error) {
	listOps := &client.ListOptions{
		FieldSelector: fields.OneTermEqualSelector(DBClusterBackupDBClusterNameField, dbName),
		Namespace:     dbNs,
	}
	backupList := &everestv1alpha1.DatabaseClusterBackupList{}
	if err := c.List(ctx, backupList, listOps); err != nil {
		return nil, errors.Join(err, errors.New("failed to list DatabaseClusterBackup objects"))
	}
	return backupList, nil
}

// ListDatabaseClusterRestores lists the DatabaseClusterRestores
// for the database specified by the name and namespace.
func ListDatabaseClusterRestores(
	ctx context.Context,
	c client.Client,
	dbName, dbNs string,
) (*everestv1alpha1.DatabaseClusterRestoreList, error) {
	restoreList := &everestv1alpha1.DatabaseClusterRestoreList{}
	listOps := &client.ListOptions{
		FieldSelector: fields.OneTermEqualSelector(DBClusterRestoreDBClusterNameField, dbName),
		Namespace:     dbNs,
	}
	if err := c.List(ctx, restoreList, listOps); err != nil {
		return nil, fmt.Errorf("failed to list DatabaseClusterRestore objects: %w", err)
	}
	return restoreList, nil
}

// GetDBMonitoringConfig returns the MonitoringConfig object
// for the given DatabaseCluster object.
func GetDBMonitoringConfig(
	ctx context.Context,
	c client.Client,
	monitoringNs string,
	database *everestv1alpha1.DatabaseCluster,
) (*everestv1alpha1.MonitoringConfig, error) {
	monitoring := &everestv1alpha1.MonitoringConfig{}
	if database.Spec.Monitoring != nil && database.Spec.Monitoring.MonitoringConfigName != "" {
		if err := c.Get(ctx, types.NamespacedName{
			Namespace: monitoringNs,
			Name:      database.Spec.Monitoring.MonitoringConfigName,
		}, monitoring); err != nil {
			return nil, err
		}
		if !monitoring.IsNamespaceAllowed(database.Namespace) {
			return nil,
				fmt.Errorf("%s namespace is not allowed to use for %s monitoring config", database.Namespace, monitoring.Name)
		}
	}
	return monitoring, nil
}

// IsDatabaseClusterRestoreRunning returns true if a restore is running for the
// specified database, otherwise false.
func IsDatabaseClusterRestoreRunning(
	ctx context.Context,
	c client.Client,
	dbName, dbNs string,
) (bool, error) {
	// List restores for this database.
	restoreList, err := ListDatabaseClusterRestores(ctx, c, dbName, dbNs)
	if err != nil {
		return false, err
	}
	// Check if any are not yet complete?
	for _, restore := range restoreList.Items {
		if !restore.IsComplete() {
			return true, nil
		}
	}
	return false, nil
}

// GetBackupStorageIndexInPGBackrestRepo returns the index of the backup storage in the pgbackrest repo list.
func GetBackupStorageIndexInPGBackrestRepo(
	backupStorage *everestv1alpha1.BackupStorage,
	repos []crunchyv1beta1.PGBackRestRepo,
) int {
	for idx, repo := range repos {
		if repo.S3 != nil &&
			repo.S3.Bucket == backupStorage.Spec.Bucket &&
			repo.S3.Region == backupStorage.Spec.Region &&
			repo.S3.Endpoint == backupStorage.Spec.EndpointURL {
			return idx
		}

		if repo.Azure != nil && repo.Azure.Container == backupStorage.Spec.Bucket {
			return idx
		}
	}
	return -1
}

// HandleUpstreamClusterCleanup handles the cleanup of the psdmb objects.
// Returns true if cleanup is complete.
func HandleUpstreamClusterCleanup(
	ctx context.Context,
	c client.Client,
	database *everestv1alpha1.DatabaseCluster,
	upstream client.Object,
) (bool, error) {
	if controllerutil.ContainsFinalizer(database, UpstreamClusterCleanupFinalizer) { //nolint:nestif
		// first check that all dbb are deleted since the upstream backups may need the upstream cluster to be present.
		backupList, err := ListDatabaseClusterBackups(ctx, c, database.GetName(), database.GetNamespace())
		if err != nil {
			return false, err
		}
		if len(backupList.Items) != 0 {
			return false, nil
		}

		err = c.Get(ctx, types.NamespacedName{
			Name:      database.Name,
			Namespace: database.Namespace,
		}, upstream)
		if err != nil {
			if k8serrors.IsNotFound(err) {
				controllerutil.RemoveFinalizer(database, UpstreamClusterCleanupFinalizer)
				return true, c.Update(ctx, database)
			}
			return false, err
		}

		err = c.Delete(ctx, upstream)
		if err != nil {
			if k8serrors.IsNotFound(err) {
				return true, nil
			}
			return false, err
		}
		controllerutil.RemoveFinalizer(database, UpstreamClusterCleanupFinalizer)
		return true, c.Update(ctx, database)
	}
	return true, nil
}

// IsOwnedBy checks if the child object is owned by the parent object.
// Returns true if child has an owner reference to the parents.
func IsOwnedBy(child, parent metav1.Object) bool {
	for _, owner := range child.GetOwnerReferences() {
		if owner.UID == parent.GetUID() {
			return true
		}
	}
	return false
}

// GetRecommendedCRVersion returns the recommended CR version for the operator.
func GetRecommendedCRVersion(
	ctx context.Context,
	c client.Client,
	operatorName string,
	db *everestv1alpha1.DatabaseCluster,
) (*string, error) {
	v, err := GetOperatorVersion(ctx, c, types.NamespacedName{
		Name:      operatorName,
		Namespace: db.GetNamespace(),
	})
	if err != nil {
		return nil, err
	}
	if v.ToCRVersion() != db.Status.CRVersion {
		return pointer.To(v.ToCRVersion()), nil
	}
	return nil, nil //nolint:nilnil
}

// ConvertToBinarySI converts a given quantity from decimal (base10) to binary (base2) SI.
// Example: 1G -> 1Gi, 2T -> 2Ti, etc.
func ConvertToBinarySI(q resource.Quantity) resource.Quantity {
	// Check if the format is a power of 10 unit (DecimalSI)
	if q.Format == resource.DecimalSI {
		value := q.Value() // byte value of the quantity.
		// c is the count of how many times `value` can be divided by 1000.
		c := int(math.Log(float64(value)) / math.Log(1000))
		// Divide value by 1000 `c` times.
		scaled := value / int64(math.Pow(1000, float64(c)))
		// Multiply value by 1024 `c` times.
		scaled = scaled * int64(math.Pow(1024, float64(c)))
		return *resource.NewQuantity(scaled, resource.BinarySI)
	}
	return q
}
