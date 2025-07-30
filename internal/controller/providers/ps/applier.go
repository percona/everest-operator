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

package ps

import (
	"context"
	"errors"
	"fmt"

	"github.com/AlekSi/pointer"
	psv1 "github.com/percona/percona-server-mysql-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	everestv1alpha1 "github.com/percona/everest-operator/api/v1alpha1"
	"github.com/percona/everest-operator/internal/consts"
	"github.com/percona/everest-operator/internal/controller/common"
)

const (
	haProxyProbesTimeout = 30
	passwordMaxLen       = 20
	passwordMinLen       = 16
	passSymbols          = "ABCDEFGHIJKLMNOPQRSTUVWXYZ" +
		"abcdefghijklmnopqrstuvwxyz" +
		"0123456789" +
		"!#$%&()+,-.<=>?@[]^_{}~"
)

var errInvalidDataSourceConfiguration = errors.New("invalid dataSource configuration")

type applier struct {
	*Provider
	ctx context.Context //nolint:containedctx
}

func (a *applier) Metadata() error {
	if a.PerconaServerMySQL.GetDeletionTimestamp().IsZero() {
		for _, f := range []string{
			finalizerDeletePSPodsInOrder,
			finalizerDeletePSPVC,
			finalizerDeletePSSSL,
		} {
			controllerutil.AddFinalizer(a.PerconaServerMySQL, f)
		}
	}
	return nil
}

func (a *applier) Paused(paused bool) {
	a.PerconaServerMySQL.Spec.Pause = paused
}

func (a *applier) AllowUnsafeConfig() {
	useInsecureSize := a.DB.Spec.Engine.Replicas == 1
	a.PerconaServerMySQL.Spec.Unsafe = psv1.UnsafeFlags{
		MySQLSize:        useInsecureSize,
		ProxySize:        useInsecureSize,
		OrchestratorSize: useInsecureSize,
	}
}

func configureStorage(
	ctx context.Context,
	c client.Client,
	desiredSpec *psv1.PerconaServerMySQLSpec,
	currentSpec *psv1.PerconaServerMySQLSpec,
	db *everestv1alpha1.DatabaseCluster,
) error {
	var currentSize resource.Quantity
	if db.Status.Status != everestv1alpha1.AppStateNew {
		currentSize = currentSpec.MySQL.PodSpec.VolumeSpec.PersistentVolumeClaim.Resources.Requests[corev1.ResourceStorage]
	}

	setStorageSize := func(size resource.Quantity) {
		desiredSpec.MySQL.PodSpec.VolumeSpec = &psv1.VolumeSpec{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimSpec{
				StorageClassName: db.Spec.Engine.Storage.Class,
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: size,
					},
				},
			},
		}
	}

	return common.ConfigureStorage(ctx, c, db, currentSize, setStorageSize)
}

// generatePass generates a random password.
// func generatePass() ([]byte, error) {
// 	randLenDelta, err := rand.Int(rand.Reader, big.NewInt(int64(passwordMaxLen-passwordMinLen)))
// 	if err != nil {
// 		return nil, err
// 	}
//
// 	b := make([]byte, passwordMinLen+randLenDelta.Int64())
// 	for i := range b {
// 		randInt, err := rand.Int(rand.Reader, big.NewInt(int64(len(passSymbols))))
// 		if err != nil {
// 			return nil, err
// 		}
// 		b[i] = passSymbols[randInt.Int64()]
// 	}
//
// 	return b, nil
// }

func (a *applier) Engine() error {
	engine := a.DBEngine
	if a.DB.Spec.Engine.Version == "" {
		a.DB.Spec.Engine.Version = engine.BestEngineVersion()
	}

	ps := a.PerconaServerMySQL

	// Update CRVersion, if specified.
	desiredCRVersion := pointer.Get(a.DB.Spec.Engine.CRVersion)
	if desiredCRVersion != "" {
		ps.Spec.CRVersion = desiredCRVersion
	}

	ps.Spec.SecretsName = a.DB.Spec.Engine.UserSecretsName
	ps.Spec.MySQL.PodSpec.Size = a.DB.Spec.Engine.Replicas
	ps.Spec.MySQL.PodSpec.Configuration = a.DB.Spec.Engine.Config

	psEngineVersion, ok := engine.Status.AvailableVersions.Engine[a.DB.Spec.Engine.Version]
	if !ok {
		return fmt.Errorf("engine version %s not available", a.DB.Spec.Engine.Version)
	}
	ps.Spec.MySQL.Image = psEngineVersion.ImagePath

	if err := configureStorage(a.ctx, a.C, &ps.Spec, &a.currentPSSpec, a.DB); err != nil {
		return err
	}

	if !a.DB.Spec.Engine.Resources.CPU.IsZero() {
		ps.Spec.MySQL.PodSpec.Resources.Limits[corev1.ResourceCPU] = a.DB.Spec.Engine.Resources.CPU
		ps.Spec.MySQL.PodSpec.Resources.Requests[corev1.ResourceCPU] = a.DB.Spec.Engine.Resources.CPU
	}
	if !a.DB.Spec.Engine.Resources.Memory.IsZero() {
		ps.Spec.MySQL.PodSpec.Resources.Limits[corev1.ResourceMemory] = a.DB.Spec.Engine.Resources.Memory
		ps.Spec.MySQL.PodSpec.Resources.Requests[corev1.ResourceMemory] = a.DB.Spec.Engine.Resources.Memory
	}
	hasDBSpecChanged := func() bool {
		return a.DB.Status.ObservedGeneration > 0 && a.DB.Status.ObservedGeneration != a.DB.Generation
	}
	// We preserve the settings for existing DBs, otherwise restarts are seen when upgrading Everest.
	// Additionally, we also need to check for the spec changes, otherwise the user can never voluntarily change the resource setting.
	// TODO: Remove this once we figure out how to apply such spec changes without automatic restarts.
	// See: https://perconadev.atlassian.net/browse/EVEREST-1413
	if a.DB.Status.Status == everestv1alpha1.AppStateReady && !hasDBSpecChanged() {
		ps.Spec.MySQL.PodSpec.Resources = a.currentPSSpec.MySQL.PodSpec.Resources
	}

	switch a.DB.Spec.Engine.Size() {
	case everestv1alpha1.EngineSizeSmall:
		ps.Spec.MySQL.PodSpec.LivenessProbe.TimeoutSeconds = 450
		ps.Spec.MySQL.PodSpec.ReadinessProbe.TimeoutSeconds = 450
	case everestv1alpha1.EngineSizeMedium:
		ps.Spec.MySQL.PodSpec.LivenessProbe.TimeoutSeconds = 451
		ps.Spec.MySQL.PodSpec.ReadinessProbe.TimeoutSeconds = 451
	case everestv1alpha1.EngineSizeLarge:
		ps.Spec.MySQL.PodSpec.LivenessProbe.TimeoutSeconds = 600
		ps.Spec.MySQL.PodSpec.ReadinessProbe.TimeoutSeconds = 600
	}

	ps.Spec.UpgradeOptions = defaultSpec().UpgradeOptions

	return nil
}

func (a *applier) Backup() error {
	bkp, err := a.genPSBackupSpec()
	if err != nil {
		return err
	}
	a.PerconaServerMySQL.Spec.Backup = bkp
	return nil
}

func (a *applier) Proxy() error {
	proxyType := a.DB.Spec.Proxy.Type
	// Apply proxy config.
	switch proxyType {
	case everestv1alpha1.ProxyTypeHAProxy:
		if err := a.applyHAProxyCfg(); err != nil {
			return err
		}
	case everestv1alpha1.ProxyTypeRouter:
		if err := a.applyRouterCfg(); err != nil {
			return err
		}
	default:
		return fmt.Errorf("invalid proxy type %s", proxyType)
	}
	return nil
}

func (a *applier) DataSource() error {
	if a.DB.Spec.DataSource == nil {
		// Nothing to do.
		return nil
	}
	// Do not restore from datasource until the cluster is ready.
	if a.DB.Status.Status != everestv1alpha1.AppStateReady {
		return nil
	}
	return common.ReconcileDBRestoreFromDataSource(a.ctx, a.C, a.DB)
}

func (a *applier) Monitoring() error {
	monitoring, err := common.GetDBMonitoringConfig(a.ctx, a.C, a.DB)
	if err != nil {
		return err
	}
	switch monitoring.Spec.Type {
	case everestv1alpha1.PMMMonitoringType:
		return a.applyPMMCfg(monitoring)
	default:
		return fmt.Errorf("invalid monitoring type %s", monitoring.Spec.Type)
	}
}

func (a *applier) PodSchedulingPolicy() error {
	// FIXME: Implement the PodSchedulingPolicy configuration.
	return nil
}

func defaultSpec() psv1.PerconaServerMySQLSpec {
	return psv1.PerconaServerMySQLSpec{
		UpdateStrategy: psv1.SmartUpdateStatefulSetStrategyType,
		UpgradeOptions: psv1.UpgradeOptions{
			Apply:                  "disabled",
			VersionServiceEndpoint: "https://check.percona.com", // FIXME: read from config
		},
		MySQL: psv1.MySQLSpec{
			ClusterType:  psv1.ClusterTypeGR,
			AutoRecovery: true,
			PodSpec: psv1.PodSpec{
				Size: 3,
				ContainerSpec: psv1.ContainerSpec{
					Resources: corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceMemory: resource.MustParse("1G"),
							corev1.ResourceCPU:    resource.MustParse("600m"),
						},
						Requests: corev1.ResourceList{
							corev1.ResourceMemory: resource.MustParse("1G"),
							corev1.ResourceCPU:    resource.MustParse("600m"),
						},
					},
				},
			},
		},
		Orchestrator: psv1.OrchestratorSpec{
			Enabled: false,
		},
		PMM: &psv1.PMMSpec{
			Enabled: false,
		},
		Backup: &psv1.BackupSpec{
			Enabled: false,
		},
		Proxy: psv1.ProxySpec{
			HAProxy: &psv1.HAProxySpec{
				Enabled: false,
				Expose: psv1.ServiceExpose{
					Type: corev1.ServiceTypeClusterIP,
				},
				PodSpec: psv1.PodSpec{
					Size: 3,
					ContainerSpec: psv1.ContainerSpec{
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								corev1.ResourceMemory: resource.MustParse("1G"),
								corev1.ResourceCPU:    resource.MustParse("600m"),
							},
							Requests: corev1.ResourceList{
								corev1.ResourceMemory: resource.MustParse("1G"),
								corev1.ResourceCPU:    resource.MustParse("600m"),
							},
						},
						ReadinessProbe: corev1.Probe{TimeoutSeconds: haProxyProbesTimeout},
						LivenessProbe:  corev1.Probe{TimeoutSeconds: haProxyProbesTimeout},
					},
				},
			},
			Router: &psv1.MySQLRouterSpec{
				Enabled: false,
				Expose: psv1.ServiceExpose{
					Type: corev1.ServiceTypeClusterIP,
				},
				PodSpec: psv1.PodSpec{
					Size: 3,
					ContainerSpec: psv1.ContainerSpec{
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{
								corev1.ResourceMemory: resource.MustParse("1G"),
								corev1.ResourceCPU:    resource.MustParse("600m"),
							},
							Requests: corev1.ResourceList{
								corev1.ResourceMemory: resource.MustParse("1G"),
								corev1.ResourceCPU:    resource.MustParse("600m"),
							},
						},
						ReadinessProbe: corev1.Probe{TimeoutSeconds: haProxyProbesTimeout},
						LivenessProbe:  corev1.Probe{TimeoutSeconds: haProxyProbesTimeout},
					},
				},
			},
		},
		TLS:     &psv1.TLSSpec{},
		Toolkit: &psv1.ToolkitSpec{},
	}
}

func (a *applier) applyHAProxyCfg() error {
	haProxy := defaultSpec().Proxy.HAProxy
	haProxy.Enabled = true

	switch a.DB.Spec.Engine.Size() {
	case everestv1alpha1.EngineSizeSmall:
		haProxy.PodSpec.Resources = haProxyResourceRequirementsSmall
	case everestv1alpha1.EngineSizeMedium:
		haProxy.PodSpec.Resources = haProxyResourceRequirementsMedium
	case everestv1alpha1.EngineSizeLarge:
		haProxy.PodSpec.Resources = haProxyResourceRequirementsLarge
	}

	if a.DB.Spec.Proxy.Replicas != nil {
		haProxy.PodSpec.Size = *a.DB.Spec.Proxy.Replicas
	} else {
		haProxy.PodSpec.Size = a.DB.Spec.Engine.Replicas
	}

	switch a.DB.Spec.Proxy.Expose.Type {
	case everestv1alpha1.ExposeTypeInternal:
		// No need to set anything, defaults are fine.
	case everestv1alpha1.ExposeTypeExternal:
		// FIXME: Check for absent key in the map.
		annotations := consts.ExposeAnnotationsMap[a.clusterType]
		expose := psv1.ServiceExpose{
			Type:                     corev1.ServiceTypeLoadBalancer,
			LoadBalancerSourceRanges: a.DB.Spec.Proxy.Expose.IPSourceRangesStringArray(),
			Annotations:              annotations,
		}
		haProxy.Expose = expose
	default:
		return fmt.Errorf("invalid expose type %s", a.DB.Spec.Proxy.Expose.Type)
	}

	if a.DB.Spec.Proxy.Config != "" {
		haProxy.PodSpec.Configuration = a.DB.Spec.Proxy.Config
	} else {
		haProxy.PodSpec.Configuration = haProxyConfigDefault
	}

	// Ensure there is an env vars secret for HAProxy
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      psHAProxyEnvSecretName,
			Namespace: a.DB.GetNamespace(),
		},
	}

	if _, err := controllerutil.CreateOrUpdate(a.ctx, a.C, secret, func() error {
		if err := controllerutil.SetOwnerReference(a.DB, secret, a.C.Scheme()); err != nil {
			return err
		}
		secret.Data = haProxyEnvVars
		return nil
	}); err != nil {
		return fmt.Errorf("failed to create or update secret %w", err)
	}
	haProxy.PodSpec.EnvFrom = []corev1.EnvFromSource{
		{
			SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: psHAProxyEnvSecretName,
				},
			},
		},
	}

	haProxyAvailVersions, ok := a.DBEngine.Status.AvailableVersions.Proxy[everestv1alpha1.ProxyTypeHAProxy]
	if !ok {
		return errors.New("haproxy version is not available")
	}
	bestHAProxyVersion := haProxyAvailVersions.BestVersion()
	haProxyVersion, ok := haProxyAvailVersions[bestHAProxyVersion]
	if !ok {
		return fmt.Errorf("haproxy version %s is not available", bestHAProxyVersion)
	}

	// We can update the HAProxy image name only in case the CRVersions match.
	// Otherwise, we keep the image unchanged.
	image := haProxyVersion.ImagePath
	if a.currentPSSpec.Proxy.HAProxy != nil && a.DBEngine.Status.OperatorVersion != a.DB.Status.CRVersion {
		image = a.currentPSSpec.Proxy.HAProxy.PodSpec.Image
	}
	haProxy.PodSpec.Image = image

	shouldUpdateRequests := shouldUpdateResourceRequests(a.DB.Status.Status)
	if !a.DB.Spec.Proxy.Resources.CPU.IsZero() {
		// When the limits are changed, triggers a pod restart, hence ensuring the requests are applied automatically (next block),
		// as it depends on the cluster being in the 'init' state (shouldUpdateRequests).
		haProxy.PodSpec.Resources.Limits[corev1.ResourceCPU] = a.DB.Spec.Proxy.Resources.CPU
		// We set the requests to the same value as the limits, however, we need to ensure that
		// they're not automatically applied when Everest is upgraded, otherwise it leads to a proxy restart.
		if shouldUpdateRequests ||
			a.currentPSSpec.Proxy.HAProxy.Resources.Requests.Cpu().
				Equal(a.DB.Spec.Proxy.Resources.CPU) {
			haProxy.PodSpec.Resources.Requests[corev1.ResourceCPU] = a.DB.Spec.Proxy.Resources.CPU
		}
	}
	if !a.DB.Spec.Proxy.Resources.Memory.IsZero() {
		// When the limits are changed, triggers a pod restart, hence ensuring the requests are applied automatically (next block),
		// as it depends on the cluster being in the 'init' state (shouldUpdateRequests).
		haProxy.PodSpec.Resources.Limits[corev1.ResourceMemory] = a.DB.Spec.Proxy.Resources.Memory
		// Prior to 1.3.0, we did not set the requests, and this led to some issues.
		// We now set the requests to the same value as the limits, however, we need to ensure that
		// they're not automatically applied when Everest is upgraded, otherwise it leads to a proxy restart.
		if shouldUpdateRequests ||
			a.currentPSSpec.Proxy.HAProxy.Resources.Requests.Memory().
				Equal(a.DB.Spec.Proxy.Resources.Memory) {
			haProxy.PodSpec.Resources.Requests[corev1.ResourceMemory] = a.DB.Spec.Proxy.Resources.Memory
		}
	}

	return nil
}

func (a *applier) applyRouterCfg() error {
	// FIXME: Implement the Router configuration.
	return nil
}

func shouldUpdateResourceRequests(dbState everestv1alpha1.AppState) bool {
	return dbState == everestv1alpha1.AppStateNew || dbState == everestv1alpha1.AppStateInit
}

// func (a *applier) applyPMMCfg(monitoring *everestv1alpha1.MonitoringConfig) error {
func (a *applier) applyPMMCfg(_ *everestv1alpha1.MonitoringConfig) error {
	// FIXME: PS supports PMM 3 only!!!
	// https://docs.percona.com/percona-operator-for-mysql/ps/monitoring.html#considerations

	// ps := a.PerconaServerMySQL
	// ps.Spec.PMM.Enabled = true
	// ps.Spec.PMM.Resources = common.GetPMMResources(pointer.Get(a.DB.Spec.Monitoring), a.DB.Spec.Engine.Size())
	//
	// if monitoring.Spec.PMM.Image != "" {
	// 	ps.Spec.PMM.Image = monitoring.Spec.PMM.Image
	// } else {
	// 	ps.Spec.PMM.Image = common.DefaultPMMClientImage
	// }
	//
	// pmmURL, err := url.Parse(monitoring.Spec.PMM.URL)
	// if err != nil {
	// 	return errors.Join(err, errors.New("invalid monitoring URL"))
	// }
	// ps.Spec.PMM.ServerHost = pmmURL.Hostname()
	//
	// apiKey, err := common.GetSecretFromMonitoringConfig(a.ctx, a.C, monitoring)
	// if err != nil {
	// 	return err
	// }
	//
	// err = common.CreateOrUpdateSecretData(a.ctx, a.C, a.DB, ps.Spec.SecretsName, map[string][]byte{
	// 	"pmmserverkey": []byte(apiKey),
	// }, false)
	// if err != nil {
	// 	return err
	// }
	return nil
}

// func (a *applier) genPXCStorageSpec(name, namespace string) (*pxcv1.BackupStorageSpec, *everestv1alpha1.BackupStorage, error) {
// 	backupStorage := &everestv1alpha1.BackupStorage{}
// 	err := a.C.Get(a.ctx, types.NamespacedName{Name: name, Namespace: namespace}, backupStorage)
// 	if err != nil {
// 		return nil, nil, errors.Join(err, fmt.Errorf("failed to get backup storage %s", name))
// 	}
//
// 	return &pxcv1.BackupStorageSpec{
// 		Type: pxcv1.BackupStorageType(backupStorage.Spec.Type),
// 		// XXX: Remove this once templates will be available
// 		Resources: corev1.ResourceRequirements{
// 			Limits: corev1.ResourceList{
// 				corev1.ResourceMemory: resource.MustParse("1G"),
// 				corev1.ResourceCPU:    resource.MustParse("600m"),
// 			},
// 		},
// 		VerifyTLS: backupStorage.Spec.VerifyTLS,
// 	}, backupStorage, nil
// }

func (a *applier) genPSBackupSpec() (*psv1.BackupSpec, error) {
	psBackupSpec := a.PerconaServerMySQL.Spec.Backup
	psBackupSpec.Enabled = len(a.DB.Spec.Backup.Schedules) > 0 ||
		a.DB.Spec.Backup.PITR.Enabled

	// Get the best backup version for the specified database engine
	bestBackupVersion := a.DBEngine.BestBackupVersion(a.DB.Spec.Engine.Version)
	backupVersion, ok := a.DBEngine.Status.AvailableVersions.Backup[bestBackupVersion]
	if !ok {
		return nil, fmt.Errorf("backup version %s is not available", bestBackupVersion)
	}

	// We can update the image name only in case the CRVersions match.
	// Otherwise we keep the image unchanged.
	if a.currentPSSpec.Backup != nil && a.DBEngine.Status.OperatorVersion != a.DB.Status.CRVersion {
		psBackupSpec.Image = a.currentPSSpec.Backup.Image
	} else {
		psBackupSpec.Image = backupVersion.ImagePath
	}

	psBackupSpec.PiTR = psv1.PiTRSpec{
		Enabled: a.DB.Spec.Backup.PITR.Enabled,
		// FIXME: enrich with more fields, like storage name, etc.
	}

	// Initialize map to store backup storages
	// storages := make(map[string]*pxcv1.BackupStorageSpec)
	//
	// // List DatabaseClusterBackup objects for this database
	// backupList, err := common.DatabaseClusterBackupsThatReferenceObject(a.ctx, a.C, consts.DBClusterBackupDBClusterNameField, database.GetNamespace(), database.GetName())
	// if err != nil {
	// 	return nil, err
	// }
	//
	// // Add the storages used by the DatabaseClusterBackup objects
	// if err = a.addBackupStorages(backupList.Items, a.DB.Spec.DataSource, storages); err != nil {
	// 	return nil, err
	// }
	//
	// // Add PITR configuration if enabled
	// if a.DB.Spec.Backup.PITR.Enabled {
	// 	if err := a.addPITRConfiguration(storages, psBackupSpec); err != nil {
	// 		return nil, err
	// 	}
	// }
	//
	// // If there are no schedules, just return the storages used in DatabaseClusterBackup objects
	// if len(a.DB.Spec.Backup.Schedules) == 0 {
	// 	psBackupSpec.Storages = storages
	// 	return psBackupSpec, nil
	// }
	//
	// // Add scheduled backup configurations
	// if err := a.addScheduledBackupsConfiguration(storages, psBackupSpec); err != nil {
	// 	return nil, err
	// }

	return psBackupSpec, nil
}

//
// func (a *applier) addBackupStorages(
// 	backups []everestv1alpha1.DatabaseClusterBackup,
// 	dataSource *everestv1alpha1.DataSource,
// 	storages map[string]*pxcv1.BackupStorageSpec,
// ) error {
// 	for _, backup := range backups {
// 		if _, ok := storages[backup.Spec.BackupStorageName]; ok {
// 			continue
// 		}
//
// 		spec, err := a.getStoragesSpec(backup.Spec.BackupStorageName)
// 		if err != nil {
// 			return err
// 		}
// 		storages[backup.Spec.BackupStorageName] = spec
// 	}
// 	// add the storage from datasource. The restore works without listing the related storage in the pxc config,
// 	// however if the storage is insecure, we need to specify it explicitly to set the insecureTLS flag
// 	if dataSource != nil && (dataSource.DBClusterBackupName != "" || dataSource.BackupSource != nil) {
// 		storageName, err := a.getStorageNameFromDataSource(*dataSource)
// 		if err != nil {
// 			return err
// 		}
// 		if _, ok := storages[storageName]; ok {
// 			return nil
// 		}
//
// 		spec, err := a.getStoragesSpec(storageName)
// 		if err != nil {
// 			return err
// 		}
//
// 		storages[storageName] = spec
// 	}
// 	return nil
// }
//
// func (a *applier) getStorageNameFromDataSource(
// 	dataSource everestv1alpha1.DataSource,
// ) (string, error) {
// 	backup := &everestv1alpha1.DatabaseClusterBackup{}
// 	var storageName string
// 	if dataSource.DBClusterBackupName != "" {
// 		err := a.C.Get(context.Background(), types.NamespacedName{
// 			Namespace: a.DB.GetNamespace(),
// 			Name:      dataSource.DBClusterBackupName,
// 		}, backup)
// 		if err != nil {
// 			return "", err
// 		}
// 		storageName = backup.Spec.BackupStorageName
// 	} else if dataSource.BackupSource != nil {
// 		storageName = dataSource.BackupSource.BackupStorageName
// 	}
// 	if storageName == "" {
// 		return "", errInvalidDataSourceConfiguration
// 	}
// 	return storageName, nil
// }
//
// func (a *applier) getStoragesSpec(backupStorageName string) (*pxcv1.BackupStorageSpec, error) {
// 	spec, backupStorage, err := a.genPXCStorageSpec(
// 		backupStorageName,
// 		a.DB.GetNamespace(),
// 	)
// 	if err != nil {
// 		return nil, errors.Join(err, fmt.Errorf("failed to generate PXC storage spec for %s", backupStorageName))
// 	}
//
// 	switch backupStorage.Spec.Type {
// 	case everestv1alpha1.BackupStorageTypeS3:
// 		spec.S3 = &pxcv1.BackupStorageS3Spec{
// 			Bucket: fmt.Sprintf(
// 				"%s/%s",
// 				backupStorage.Spec.Bucket,
// 				common.BackupStoragePrefix(a.DB),
// 			),
// 			CredentialsSecret: backupStorage.Spec.CredentialsSecretName,
// 			Region:            backupStorage.Spec.Region,
// 			EndpointURL:       backupStorage.Spec.EndpointURL,
// 		}
// 	case everestv1alpha1.BackupStorageTypeAzure:
// 		spec.Azure = &pxcv1.BackupStorageAzureSpec{
// 			ContainerPath: fmt.Sprintf(
// 				"%s/%s",
// 				backupStorage.Spec.Bucket,
// 				common.BackupStoragePrefix(a.DB),
// 			),
// 			CredentialsSecret: backupStorage.Spec.CredentialsSecretName,
// 		}
// 	default:
// 		return nil, fmt.Errorf("unsupported backup storage type %s for %s", backupStorage.Spec.Type, backupStorage.Name)
// 	}
// 	return spec, nil
// }
//
// func (a *applier) addPITRConfiguration(storages map[string]*pxcv1.BackupStorageSpec, pxcBackupSpec *pxcv1.PXCScheduledBackup) error {
// 	database := a.DB
// 	storageName := *database.Spec.Backup.PITR.BackupStorageName
//
// 	spec, backupStorage, err := a.genPXCStorageSpec(storageName, database.GetNamespace())
// 	if err != nil {
// 		return errors.Join(err, errors.New("failed to get pitr storage"))
// 	}
// 	pxcBackupSpec.PITR.StorageName = common.PITRStorageName(storageName)
//
// 	var timeBetweenUploads float64
// 	if database.Spec.Backup.PITR.UploadIntervalSec != nil {
// 		timeBetweenUploads = float64(*database.Spec.Backup.PITR.UploadIntervalSec)
// 	}
// 	pxcBackupSpec.PITR.TimeBetweenUploads = timeBetweenUploads
//
// 	switch backupStorage.Spec.Type {
// 	case everestv1alpha1.BackupStorageTypeS3:
// 		spec.S3 = &pxcv1.BackupStorageS3Spec{
// 			Bucket:            common.PITRBucketName(database, backupStorage.Spec.Bucket),
// 			CredentialsSecret: backupStorage.Spec.CredentialsSecretName,
// 			Region:            backupStorage.Spec.Region,
// 			EndpointURL:       backupStorage.Spec.EndpointURL,
// 		}
// 	default:
// 		return fmt.Errorf("BackupStorage of type %s is not supported. PITR only works for s3 compatible storages", backupStorage.Spec.Type)
// 	}
//
// 	// create a separate storage for pxc pitr as the docs recommend
// 	// https://docs.percona.com/percona-operator-for-mysql/pxc/backups-pitr.html
// 	storages[common.PITRStorageName(backupStorage.Name)] = spec
// 	return nil
// }
//
// func (a *applier) addScheduledBackupsConfiguration(
// 	storages map[string]*pxcv1.BackupStorageSpec,
// 	pxcBackupSpec *pxcv1.PXCScheduledBackup,
// ) error {
// 	database := a.DB
// 	var pxcSchedules []pxcv1.PXCScheduledBackupSchedule //nolint:prealloc
// 	for _, schedule := range database.Spec.Backup.Schedules {
// 		if !schedule.Enabled {
// 			continue
// 		}
//
// 		// Add the storages used by the schedule backups
// 		if _, ok := storages[schedule.BackupStorageName]; !ok {
// 			backupStorage := &everestv1alpha1.BackupStorage{}
// 			err := a.C.Get(a.ctx, types.NamespacedName{
// 				Name:      schedule.BackupStorageName,
// 				Namespace: database.GetNamespace(),
// 			}, backupStorage)
// 			if err != nil {
// 				return errors.Join(err, fmt.Errorf("failed to get backup storage %s", schedule.BackupStorageName))
// 			}
//
// 			storages[schedule.BackupStorageName] = &pxcv1.BackupStorageSpec{
// 				Type:      pxcv1.BackupStorageType(backupStorage.Spec.Type),
// 				VerifyTLS: backupStorage.Spec.VerifyTLS,
// 			}
// 			switch backupStorage.Spec.Type {
// 			case everestv1alpha1.BackupStorageTypeS3:
// 				storages[schedule.BackupStorageName].S3 = &pxcv1.BackupStorageS3Spec{
// 					Bucket: fmt.Sprintf(
// 						"%s/%s",
// 						backupStorage.Spec.Bucket,
// 						common.BackupStoragePrefix(database),
// 					),
// 					CredentialsSecret: backupStorage.Spec.CredentialsSecretName,
// 					Region:            backupStorage.Spec.Region,
// 					EndpointURL:       backupStorage.Spec.EndpointURL,
// 				}
// 			case everestv1alpha1.BackupStorageTypeAzure:
// 				storages[schedule.BackupStorageName].Azure = &pxcv1.BackupStorageAzureSpec{
// 					ContainerPath: fmt.Sprintf(
// 						"%s/%s",
// 						backupStorage.Spec.Bucket,
// 						common.BackupStoragePrefix(database),
// 					),
// 					CredentialsSecret: backupStorage.Spec.CredentialsSecretName,
// 				}
// 			default:
// 				return fmt.Errorf("unsupported backup storage type %s for %s", backupStorage.Spec.Type, backupStorage.Name)
// 			}
// 		}
//
// 		pxcSchedules = append(pxcSchedules, pxcv1.PXCScheduledBackupSchedule{
// 			Name:        schedule.Name,
// 			Schedule:    schedule.Schedule,
// 			Keep:        int(schedule.RetentionCopies),
// 			StorageName: schedule.BackupStorageName,
// 		})
// 	}
//
// 	pxcBackupSpec.Storages = storages
// 	pxcBackupSpec.Schedule = pxcSchedules
// 	return nil
// }
