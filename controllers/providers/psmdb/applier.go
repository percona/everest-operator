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

package psmdb

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"time"

	"github.com/AlekSi/pointer"
	psmdbv1 "github.com/percona/percona-server-mongodb-operator/pkg/apis/psmdb/v1"
	"github.com/percona/percona-server-mongodb-operator/pkg/util/numstr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"

	everestv1alpha1 "github.com/percona/everest-operator/api/v1alpha1"
	"github.com/percona/everest-operator/controllers/common"
)

const (
	psmdbDefaultConfigurationTemplate = `
      operationProfiling:
        mode: slowOp
`
	encryptionKeySuffix = "-mongodb-encryption-key"
)

type applier struct {
	*Provider
	ctx context.Context //nolint:containedctx
}

func (p *applier) Paused(paused bool) {
	p.PerconaServerMongoDB.Spec.Pause = paused
}

func (p *applier) AllowUnsafeConfig(allow bool) {
	p.PerconaServerMongoDB.Spec.UnsafeConf = allow
}

func (p *applier) Engine() error {
	psmdb := p.PerconaServerMongoDB
	database := p.DB
	engine := p.DBEngine

	// Update CRVersion, if specified.
	desiredCR := pointer.Get(p.DB.Spec.Engine.CRVersion)
	if desiredCR != "" {
		psmdb.Spec.CRVersion = desiredCR
	}

	if database.Spec.Engine.Version == "" {
		database.Spec.Engine.Version = engine.BestEngineVersion()
	}
	engineVersion, ok := engine.Status.AvailableVersions.Engine[database.Spec.Engine.Version]
	if !ok {
		return fmt.Errorf("engine version %s not available", database.Spec.Engine.Version)
	}
	psmdb.Spec.Image = engineVersion.ImagePath
	psmdb.Spec.Secrets = &psmdbv1.SecretsSpec{
		Users:         database.Spec.Engine.UserSecretsName,
		EncryptionKey: database.Name + encryptionKeySuffix,
	}

	p.configureSharding()

	return nil
}

func (p *applier) configureSharding() {
	database := p.DB
	psmdb := p.PerconaServerMongoDB

	// TODO: implement disabling
	if database.Spec.Sharding == nil || !database.Spec.Sharding.Enabled {
		// keep the default configuration
		p.configureReplSetSpec(psmdb.Spec.Replsets[0], rsName(0))
		return
	}

	psmdb.Spec.Sharding.Enabled = database.Spec.Sharding.Enabled

	// Add replsets if needed to fit the shards number
	// TODO: implement scale down
	shards := int(database.Spec.Sharding.Shards)
	for i := len(psmdb.Spec.Replsets); i < shards; i++ {
		rs0copy := *psmdb.Spec.Replsets[0]
		psmdb.Spec.Replsets = append(psmdb.Spec.Replsets, &rs0copy)
	}

	// configure all replsets
	for i := 0; i < shards; i++ {
		p.configureReplSetSpec(psmdb.Spec.Replsets[i], rsName(i))
	}

	if psmdb.Spec.Sharding.ConfigsvrReplSet == nil {
		psmdb.Spec.Sharding.ConfigsvrReplSet = &psmdbv1.ReplsetSpec{}
		p.configureConfigsvrReplSet(psmdb.Spec.Sharding.ConfigsvrReplSet)
	}
}

func rsName(i int) string {
	return fmt.Sprintf("rs%v", i)
}

func (p *applier) configureConfigsvrReplSet(configsvr *psmdbv1.ReplsetSpec) {
	database := p.DB
	configsvr.Size = database.Spec.Sharding.ConfigServer.Replicas
	configsvr.MultiAZ.Affinity = &psmdbv1.PodAffinity{
		Advanced: common.DefaultAffinitySettings().DeepCopy(),
	}
	configsvr.VolumeSpec = &psmdbv1.VolumeSpec{
		PersistentVolumeClaim: psmdbv1.PVCSpec{
			PersistentVolumeClaimSpec: &corev1.PersistentVolumeClaimSpec{
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: p.DB.Spec.Engine.Storage.Size,
					},
				},
			},
		},
	}
}

func (p *applier) configureReplSetSpec(spec *psmdbv1.ReplsetSpec, name string) {
	spec.Name = name
	dbEngine := p.DB.Spec.Engine
	if dbEngine.Config != "" {
		spec.Configuration = psmdbv1.MongoConfiguration(dbEngine.Config)
	}
	if spec.Configuration == "" {
		// Config missing from the DatabaseCluster CR and the template (if any), apply the default one
		spec.Configuration = psmdbv1.MongoConfiguration(psmdbDefaultConfigurationTemplate)
	}

	affinity := &psmdbv1.PodAffinity{
		Advanced: common.DefaultAffinitySettings().DeepCopy(),
	}
	spec.MultiAZ.Affinity = affinity
	spec.Size = dbEngine.Replicas
	spec.VolumeSpec = &psmdbv1.VolumeSpec{
		PersistentVolumeClaim: psmdbv1.PVCSpec{
			PersistentVolumeClaimSpec: &corev1.PersistentVolumeClaimSpec{
				StorageClassName: dbEngine.Storage.Class,
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: dbEngine.Storage.Size,
					},
				},
			},
		},
	}
	if !dbEngine.Resources.CPU.IsZero() {
		spec.MultiAZ.Resources.Limits[corev1.ResourceCPU] = dbEngine.Resources.CPU
	}
	if !dbEngine.Resources.Memory.IsZero() {
		spec.MultiAZ.Resources.Limits[corev1.ResourceMemory] = dbEngine.Resources.Memory
	}
}

func (p *applier) Proxy() error {
	psmdb := p.PerconaServerMongoDB
	database := p.DB

	// if sharding is disabled, expose the default replset directly as usual according to db proxy settings
	if database.Spec.Sharding == nil || !database.Spec.Sharding.Enabled {
		psmdb.Spec.Sharding.Enabled = false
		err := p.exposeDefaultReplSet(&psmdb.Spec.Replsets[0].Expose)
		if err != nil {
			return err
		}
	}

	// otherwise configure psmdb.Spec.Sharding.Mongos according to the db proxy settings
	if psmdb.Spec.Sharding.Mongos == nil {
		var size = database.Spec.Engine.Replicas
		if database.Spec.Proxy.Replicas != nil {
			size = *database.Spec.Proxy.Replicas
		}
		psmdb.Spec.Sharding.Mongos = &psmdbv1.MongosSpec{
			Size: size,
			MultiAZ: psmdbv1.MultiAZ{
				Affinity: &psmdbv1.PodAffinity{
					Advanced: common.DefaultAffinitySettings().DeepCopy(),
				},
			},
		}
	}
	err := p.exposeShardedCluster(&psmdb.Spec.Sharding.Mongos.Expose)
	if err != nil {
		return err
	}

	// disable direct exposure of replsets since .psmdb.Spec.Sharding.Mongos works like proxy
	psmdb.Spec.Replsets[0].Expose.Enabled = false
	psmdb.Spec.Replsets[0].Expose.ExposeType = corev1.ServiceTypeClusterIP
	return nil
}

func (p *applier) exposeShardedCluster(expose *psmdbv1.MongosExpose) error {
	database := p.DB
	switch database.Spec.Proxy.Expose.Type {
	case everestv1alpha1.ExposeTypeInternal:
		expose.ExposeType = corev1.ServiceTypeClusterIP
	case everestv1alpha1.ExposeTypeExternal:
		expose.ExposeType = corev1.ServiceTypeLoadBalancer
		expose.LoadBalancerSourceRanges = database.Spec.Proxy.Expose.IPSourceRangesStringArray()
		if annotations, ok := common.ExposeAnnotationsMap[p.clusterType]; ok {
			expose.ServiceAnnotations = annotations
		}
	default:
		return fmt.Errorf("invalid expose type %s", database.Spec.Proxy.Expose.Type)
	}
	return nil
}

func (p *applier) exposeDefaultReplSet(expose *psmdbv1.ExposeTogglable) error {
	database := p.DB
	switch database.Spec.Proxy.Expose.Type {
	case everestv1alpha1.ExposeTypeInternal:
		expose.Enabled = false
		expose.ExposeType = corev1.ServiceTypeClusterIP
	case everestv1alpha1.ExposeTypeExternal:
		expose.Enabled = true
		expose.ExposeType = corev1.ServiceTypeLoadBalancer
		expose.LoadBalancerSourceRanges = database.Spec.Proxy.Expose.IPSourceRangesStringArray()
		if annotations, ok := common.ExposeAnnotationsMap[p.clusterType]; ok {
			expose.ServiceAnnotations = annotations
		}
	default:
		return fmt.Errorf("invalid expose type %s", database.Spec.Proxy.Expose.Type)
	}
	return nil
}

func (p *applier) Backup() error {
	spec, err := p.genPSMDBBackupSpec()
	if err != nil {
		return err
	}
	p.PerconaServerMongoDB.Spec.Backup = spec
	return nil
}

func (p *applier) DataSource() error {
	database := p.DB
	if database.Spec.DataSource == nil {
		// Nothing to do.
		return nil
	}
	if database.Status.Status != everestv1alpha1.AppStateReady {
		// Wait for the cluster to be ready.
		return nil
	}
	return common.ReconcileDBRestoreFromDataSource(p.ctx, p.C, p.DB)
}

func (p *applier) Monitoring() error {
	monitoring, err := common.GetDBMonitoringConfig(p.ctx, p.C, p.DB)
	if err != nil {
		return err
	}
	p.PerconaServerMongoDB.Spec.PMM = defaultSpec().PMM
	if monitoring.Spec.Type == everestv1alpha1.PMMMonitoringType {
		if err := p.applyPMMCfg(monitoring); err != nil {
			return err
		}
	}
	return nil
}

func (p *applier) applyPMMCfg(monitoring *everestv1alpha1.MonitoringConfig) error {
	psmdb := p.PerconaServerMongoDB
	database := p.DB
	ctx := p.ctx
	c := p.C

	image := common.DefaultPMMClientImage
	if monitoring.Spec.PMM.Image != "" {
		image = monitoring.Spec.PMM.Image
	}

	psmdb.Spec.PMM.Enabled = true
	pmmURL, err := url.Parse(monitoring.Spec.PMM.URL)
	if err != nil {
		return errors.Join(err, errors.New("invalid monitoring URL"))
	}
	psmdb.Spec.PMM.ServerHost = pmmURL.Hostname()
	psmdb.Spec.PMM.Image = image
	psmdb.Spec.PMM.Resources = database.Spec.Monitoring.Resources

	apiKey, err := common.GetSecretFromMonitoringConfig(ctx, c, monitoring)
	if err != nil {
		return err
	}

	if err := common.CreateOrUpdateSecretData(ctx, c, database, psmdb.Spec.Secrets.Users,
		map[string][]byte{
			"PMM_SERVER_API_KEY": []byte(apiKey),
		},
	); err != nil {
		return err
	}
	return nil
}

// Add the storages used by restores.
func (p *applier) addBackupStoragesByRestores(
	restoreList *everestv1alpha1.DatabaseClusterRestoreList,
	storages map[string]psmdbv1.BackupStorageSpec,
) error {
	database := p.DB
	ctx := p.ctx
	c := p.C
	// Add used restore backup storages to the list
	for _, restore := range restoreList.Items {
		// If the restore has already completed, skip it.
		if restore.IsComplete() {
			continue
		}
		// Restores using the BackupSource field instead of the
		// DBClusterBackupName don't need to have the storage defined, skip
		// them.
		if restore.Spec.DataSource.DBClusterBackupName == "" {
			continue
		}

		backup := &everestv1alpha1.DatabaseClusterBackup{}
		key := types.NamespacedName{Name: restore.Spec.DataSource.DBClusterBackupName, Namespace: restore.GetNamespace()}
		err := c.Get(ctx, key, backup)
		if err != nil {
			return errors.Join(err, errors.New("failed to get DatabaseClusterBackup"))
		}

		// Check if we already fetched that backup storage
		if _, ok := storages[backup.Spec.BackupStorageName]; ok {
			continue
		}

		backupStorage, err := common.GetBackupStorage(ctx, c, backup.Spec.BackupStorageName, database.GetNamespace())
		if err != nil {
			return err
		}

		// configures the storage for S3.
		handleStorageS3 := func() {
			storages[backup.Spec.BackupStorageName] = psmdbv1.BackupStorageSpec{
				Type: psmdbv1.BackupStorageS3,
				S3: psmdbv1.BackupStorageS3Spec{
					Bucket:                backupStorage.Spec.Bucket,
					CredentialsSecret:     backupStorage.Spec.CredentialsSecretName,
					Region:                backupStorage.Spec.Region,
					EndpointURL:           backupStorage.Spec.EndpointURL,
					Prefix:                common.BackupStoragePrefix(database),
					InsecureSkipTLSVerify: !pointer.Get(backupStorage.Spec.VerifyTLS),
				},
			}
		}
		// configures the storage for Azure.
		handleStorageAzure := func() {
			storages[backup.Spec.BackupStorageName] = psmdbv1.BackupStorageSpec{
				Type: psmdbv1.BackupStorageAzure,
				Azure: psmdbv1.BackupStorageAzureSpec{
					Container:         backupStorage.Spec.Bucket,
					Prefix:            common.BackupStoragePrefix(database),
					CredentialsSecret: backupStorage.Spec.CredentialsSecretName,
				},
			}
		}

		switch backupStorage.Spec.Type {
		case everestv1alpha1.BackupStorageTypeS3:
			handleStorageS3()
		case everestv1alpha1.BackupStorageTypeAzure:
			handleStorageAzure()
		default:
			return fmt.Errorf("unsupported backup storage type %s for %s", backupStorage.Spec.Type, backupStorage.Name)
		}
	}
	return nil
}

// Add the storages used by the DatabaseClusterBackup objects.
func (p *applier) addBackupStoragesByDatabaseClusterBackups(
	backupList *everestv1alpha1.DatabaseClusterBackupList,
	storages map[string]psmdbv1.BackupStorageSpec,
) error {
	database := p.DB
	ctx := p.ctx
	c := p.C

	for _, backup := range backupList.Items {
		// Check if we already fetched that backup storage
		if _, ok := storages[backup.Spec.BackupStorageName]; ok {
			continue
		}

		backupStorage, err := common.GetBackupStorage(ctx, c, backup.Spec.BackupStorageName, database.GetNamespace())
		if err != nil {
			return err
		}

		// configures the storage for S3.
		handleStorageS3 := func() {
			storages[backup.Spec.BackupStorageName] = psmdbv1.BackupStorageSpec{
				Type: psmdbv1.BackupStorageS3,
				S3: psmdbv1.BackupStorageS3Spec{
					Bucket:                backupStorage.Spec.Bucket,
					CredentialsSecret:     backupStorage.Spec.CredentialsSecretName,
					Region:                backupStorage.Spec.Region,
					EndpointURL:           backupStorage.Spec.EndpointURL,
					Prefix:                common.BackupStoragePrefix(database),
					InsecureSkipTLSVerify: !pointer.Get(backupStorage.Spec.VerifyTLS),
				},
			}
		}
		// configures the storage for Azure.
		handleStorageAzure := func() {
			storages[backup.Spec.BackupStorageName] = psmdbv1.BackupStorageSpec{
				Type: psmdbv1.BackupStorageAzure,
				Azure: psmdbv1.BackupStorageAzureSpec{
					Container:         backupStorage.Spec.Bucket,
					Prefix:            common.BackupStoragePrefix(database),
					CredentialsSecret: backupStorage.Spec.CredentialsSecretName,
				},
			}
		}

		switch backupStorage.Spec.Type {
		case everestv1alpha1.BackupStorageTypeS3:
			handleStorageS3()
		case everestv1alpha1.BackupStorageTypeAzure:
			handleStorageAzure()
		default:
			return fmt.Errorf("unsupported backup storage type %s for %s", backupStorage.Spec.Type, backupStorage.Name)
		}
	}
	return nil
}

func (p *applier) getBackupTasks(
	storages map[string]psmdbv1.BackupStorageSpec,
) ([]psmdbv1.BackupTaskSpec, error) {
	database := p.DB
	c := p.C
	ctx := p.ctx

	tasks := make([]psmdbv1.BackupTaskSpec, 0, len(database.Spec.Backup.Schedules))
	for _, schedule := range database.Spec.Backup.Schedules {
		if !schedule.Enabled {
			continue
		}

		backupStorage, err := common.GetBackupStorage(ctx, c, schedule.BackupStorageName, database.GetNamespace())
		if err != nil {
			return nil, err
		}

		// configures the storage for S3.
		handleStorageS3 := func() {
			storages[schedule.BackupStorageName] = psmdbv1.BackupStorageSpec{
				Type: psmdbv1.BackupStorageS3,
				S3: psmdbv1.BackupStorageS3Spec{
					Bucket:                backupStorage.Spec.Bucket,
					CredentialsSecret:     backupStorage.Spec.CredentialsSecretName,
					Region:                backupStorage.Spec.Region,
					EndpointURL:           backupStorage.Spec.EndpointURL,
					Prefix:                common.BackupStoragePrefix(database),
					InsecureSkipTLSVerify: !pointer.Get(backupStorage.Spec.VerifyTLS),
				},
			}
		}
		// configures the storage for Azure.
		handleStorageAzure := func() {
			storages[schedule.BackupStorageName] = psmdbv1.BackupStorageSpec{
				Type: psmdbv1.BackupStorageAzure,
				Azure: psmdbv1.BackupStorageAzureSpec{
					Container:         backupStorage.Spec.Bucket,
					Prefix:            common.BackupStoragePrefix(database),
					CredentialsSecret: backupStorage.Spec.CredentialsSecretName,
				},
			}
		}

		switch backupStorage.Spec.Type {
		case everestv1alpha1.BackupStorageTypeS3:
			handleStorageS3()
		case everestv1alpha1.BackupStorageTypeAzure:
			handleStorageAzure()
		default:
			return nil, fmt.Errorf("unsupported backup storage type %s for %s", backupStorage.Spec.Type, backupStorage.Name)
		}

		tasks = append(tasks, psmdbv1.BackupTaskSpec{
			Name:        schedule.Name,
			Enabled:     true,
			Schedule:    schedule.Schedule,
			Keep:        int(schedule.RetentionCopies),
			StorageName: schedule.BackupStorageName,
		})
	}
	return tasks, nil
}

func (p *applier) genPSMDBBackupSpec() (psmdbv1.BackupSpec, error) {
	database := p.DB
	ctx := p.ctx
	engine := p.DBEngine
	c := p.C

	emptySpec := psmdbv1.BackupSpec{Enabled: false}

	// Verify backup version.
	bestBackupVersion := engine.BestBackupVersion(database.Spec.Engine.Version)
	backupVersion, ok := engine.Status.AvailableVersions.Backup[bestBackupVersion]
	if !ok {
		return emptySpec, fmt.Errorf("backup version %s not available", bestBackupVersion)
	}

	// Initialize backup spec.
	psmdbBackupSpec := psmdbv1.BackupSpec{
		Enabled: true,
		Image:   backupVersion.ImagePath,
		PITR: psmdbv1.PITRSpec{
			Enabled: database.Spec.Backup.PITR.Enabled,
		},

		// XXX: Remove this once templates will be available
		Resources: corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("1G"),
				corev1.ResourceCPU:    resource.MustParse("300m"),
			},
		},
	}

	storages := make(map[string]psmdbv1.BackupStorageSpec)

	// List DatabaseClusterBackup objects for this database
	backupList, err := common.ListDatabaseClusterBackups(ctx, c, database.GetName(), database.GetNamespace())
	if err != nil {
		return emptySpec, err
	}
	// Add the storages used by the DatabaseClusterBackup objects
	if err := p.addBackupStoragesByDatabaseClusterBackups(backupList, storages); err != nil {
		return emptySpec, err
	}

	// List DatabaseClusterRestore objects for this database
	restoreList, err := common.ListDatabaseClusterRestores(ctx, c, database.GetName(), database.GetNamespace())
	if err != nil {
		return emptySpec, err
	}
	// Add the storages used by restores.
	if err := p.addBackupStoragesByRestores(restoreList, storages); err != nil {
		return emptySpec, err
	}

	// If scheduled backups are disabled, just return the storages used in
	// DatabaseClusterBackup objects
	if !database.Spec.Backup.Enabled {
		if len(storages) > 1 {
			return emptySpec, common.ErrPSMDBOneStorageRestriction
		}
		psmdbBackupSpec.Storages = storages
		return psmdbBackupSpec, nil
	}

	tasks, err := p.getBackupTasks(storages)
	if err != nil {
		return emptySpec, err
	}

	if database.Spec.Backup.PITR.Enabled {
		interval := database.Spec.Backup.PITR.UploadIntervalSec
		if interval != nil {
			// the integer amount of minutes. default 10
			intervalMinutes := *interval / int(time.Minute.Seconds())
			psmdbBackupSpec.PITR.OplogSpanMin = numstr.NumberString(strconv.Itoa(intervalMinutes))
		}
	}

	if len(storages) > 1 {
		return emptySpec, common.ErrPSMDBOneStorageRestriction
	}

	psmdbBackupSpec.Storages = storages
	psmdbBackupSpec.Tasks = tasks

	return psmdbBackupSpec, nil
}
