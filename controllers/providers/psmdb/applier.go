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
	if database.Spec.Engine.Config != "" {
		psmdb.Spec.Replsets[0].Configuration = psmdbv1.MongoConfiguration(database.Spec.Engine.Config)
	}
	if psmdb.Spec.Replsets[0].Configuration == "" {
		// Config missing from the DatabaseCluster CR and the template (if any), apply the default one
		psmdb.Spec.Replsets[0].Configuration = psmdbv1.MongoConfiguration(psmdbDefaultConfigurationTemplate)
	}

	if p.clusterType == common.ClusterTypeEKS {
		affinity := &psmdbv1.PodAffinity{
			TopologyKey: pointer.ToString("kubernetes.io/hostname"),
		}
		psmdb.Spec.Replsets[0].MultiAZ.Affinity = affinity
	}

	psmdb.Spec.Replsets[0].Size = database.Spec.Engine.Replicas
	psmdb.Spec.Replsets[0].VolumeSpec = &psmdbv1.VolumeSpec{
		PersistentVolumeClaim: psmdbv1.PVCSpec{
			PersistentVolumeClaimSpec: &corev1.PersistentVolumeClaimSpec{
				StorageClassName: database.Spec.Engine.Storage.Class,
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: database.Spec.Engine.Storage.Size,
					},
				},
			},
		},
	}
	if !database.Spec.Engine.Resources.CPU.IsZero() {
		psmdb.Spec.Replsets[0].MultiAZ.Resources.Limits[corev1.ResourceCPU] = database.Spec.Engine.Resources.CPU
	}
	if !database.Spec.Engine.Resources.Memory.IsZero() {
		psmdb.Spec.Replsets[0].MultiAZ.Resources.Limits[corev1.ResourceMemory] = database.Spec.Engine.Resources.Memory
	}
	return nil
}

func (p *applier) Proxy() error {
	psmdb := p.PerconaServerMongoDB
	database := p.DB
	switch database.Spec.Proxy.Expose.Type {
	case everestv1alpha1.ExposeTypeInternal:
		psmdb.Spec.Replsets[0].Expose.Enabled = false
		psmdb.Spec.Replsets[0].Expose.ExposeType = corev1.ServiceTypeClusterIP
	case everestv1alpha1.ExposeTypeExternal:
		psmdb.Spec.Replsets[0].Expose.Enabled = true
		psmdb.Spec.Replsets[0].Expose.ExposeType = corev1.ServiceTypeLoadBalancer
		psmdb.Spec.Replsets[0].Expose.LoadBalancerSourceRanges = database.Spec.Proxy.Expose.IPSourceRangesStringArray()
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
	return common.ReconcileDBRestoreFromDataSource(p.ctx, p.C, p.DB, p.C.Get)
}

func (p *applier) Monitoring() error {
	monitoring, err := common.GetDBMonitoringConfig(p.ctx, p.C, p.MonitoringNs, p.DB)
	if err != nil {
		return err
	}
	p.PerconaServerMongoDB.Spec.PMM = defaultSpec().PMM
	switch monitoring.Spec.Type {
	case everestv1alpha1.PMMMonitoringType:
		if err := p.applyPMMCfg(monitoring); err != nil {
			return err
		}
	default:
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

	apiKey, err := common.GetSecretFromMonitoringConfig(ctx, c, monitoring, p.MonitoringNs)
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
		if restore.IsComplete(database.Spec.Engine.Type) {
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

		backupStorage, err := common.GetBackupStorage(ctx, c, backup.Spec.BackupStorageName, p.SystemNs)
		if err != nil {
			return err
		}
		if database.Namespace != p.SystemNs {
			if err := common.ReconcileBackupStorageSecret(ctx, c, p.SystemNs, backupStorage, database); err != nil {
				return err
			}
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

		backupStorage, err := common.GetBackupStorage(ctx, c, backup.Spec.BackupStorageName, p.SystemNs)
		if err != nil {
			return err
		}
		if database.GetNamespace() != p.SystemNs {
			if err := common.ReconcileBackupStorageSecret(ctx, c, p.SystemNs, backupStorage, database); err != nil {
				return err
			}
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

		backupStorage, err := common.GetBackupStorage(ctx, c, schedule.BackupStorageName, p.SystemNs)
		if err != nil {
			return nil, err
		}
		if database.GetNamespace() != p.SystemNs {
			if err := common.ReconcileBackupStorageSecret(ctx, c, p.SystemNs, backupStorage, database); err != nil {
				return nil, err
			}
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
			psmdbBackupSpec.PITR.OplogSpanMin = numstr.NumberString(strconv.Itoa(*interval / 60))
		}
	}

	if len(storages) > 1 {
		return emptySpec, common.ErrPSMDBOneStorageRestriction
	}

	psmdbBackupSpec.Storages = storages
	psmdbBackupSpec.Tasks = tasks

	return psmdbBackupSpec, nil
}
