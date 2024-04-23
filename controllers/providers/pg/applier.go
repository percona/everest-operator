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
	"bytes"
	"container/list"
	"context"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/AlekSi/pointer"
	"github.com/go-ini/ini"
	pgv2 "github.com/percona/percona-postgresql-operator/pkg/apis/pgv2.percona.com/v2"
	crunchyv1beta1 "github.com/percona/percona-postgresql-operator/pkg/apis/postgres-operator.crunchydata.com/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	everestv1alpha1 "github.com/percona/everest-operator/api/v1alpha1"
	"github.com/percona/everest-operator/controllers/common"
)

const (
	pgBackupTypeDate      = "time"
	pgBackupTypeImmediate = "immediate"

	pgBackRestPathTmpl             = "%s-path"
	pgBackRestRetentionTmpl        = "%s-retention-full"
	pgBackRestStorageVerifyTmpl    = "%s-storage-verify-tls"
	pgBackRestStorageForcePathTmpl = "%s-s3-uri-style"
	pgBackRestStoragePathStyle     = "path"
)

type applier struct {
	*Provider
	ctx context.Context //nolint:containedctx
}

func (p *applier) Paused(paused bool) {
	p.PerconaPGCluster.Spec.Pause = &paused
}

func (p *applier) AllowUnsafeConfig(_ bool) {
}

func (p *applier) Engine() error {
	pg := p.PerconaPGCluster
	database := p.DB
	engine := p.DBEngine

	if database.Spec.Engine.Version == "" {
		database.Spec.Engine.Version = engine.BestEngineVersion()
	}

	// Set a CRVersion.
	observedCRVersion := p.DB.Status.CRVersion
	desiredCRVersion := pointer.Get(p.DB.Spec.Engine.CRVersion)
	pg.Spec.CRVersion = observedCRVersion
	if observedCRVersion == "" && desiredCRVersion == "" {
		// During the initial installation, a CRVersion may not be provided.
		// So we will use the operator version.
		v, err := common.GetOperatorVersion(p.ctx, p.C, types.NamespacedName{
			Name:      common.PGDeploymentName,
			Namespace: p.DB.GetNamespace(),
		})
		if err != nil {
			return err
		}
		pg.Spec.CRVersion = v.ToCRVersion()
	} else if desiredCRVersion != "" {
		// Otherwise, we always use the one provided.
		pg.Spec.CRVersion = desiredCRVersion
	}

	pgEngineVersion, ok := engine.Status.AvailableVersions.Engine[database.Spec.Engine.Version]
	if !ok {
		return fmt.Errorf("engine version %s not available", database.Spec.Engine.Version)
	}
	pg.Spec.Image = pgEngineVersion.ImagePath
	pgMajorVersionMatch := regexp.
		MustCompile(`^(\d+)`).
		FindStringSubmatch(database.Spec.Engine.Version)
	if len(pgMajorVersionMatch) < 2 {
		return fmt.Errorf("failed to extract the major version from %s", database.Spec.Engine.Version)
	}
	pgMajorVersion, err := strconv.Atoi(pgMajorVersionMatch[1])
	if err != nil {
		return err
	}
	pg.Spec.PostgresVersion = pgMajorVersion
	if err := p.updatePGConfig(pg, database); err != nil {
		return errors.Join(err, errors.New("could not update PG config"))
	}
	pg.Spec.InstanceSets[0].Replicas = &database.Spec.Engine.Replicas
	if !database.Spec.Engine.Resources.CPU.IsZero() {
		pg.Spec.InstanceSets[0].Resources.Limits[corev1.ResourceCPU] = database.Spec.Engine.Resources.CPU
	}
	if !database.Spec.Engine.Resources.Memory.IsZero() {
		pg.Spec.InstanceSets[0].Resources.Limits[corev1.ResourceMemory] = database.Spec.Engine.Resources.Memory
	}
	pg.Spec.InstanceSets[0].DataVolumeClaimSpec = corev1.PersistentVolumeClaimSpec{
		AccessModes: []corev1.PersistentVolumeAccessMode{
			corev1.ReadWriteOnce,
		},
		StorageClassName: database.Spec.Engine.Storage.Class,
		Resources: corev1.VolumeResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceStorage: database.Spec.Engine.Storage.Size,
			},
		},
	}
	if p.clusterType == common.ClusterTypeEKS {
		pg.Spec.InstanceSets[0].Affinity = hostnameAffinity.DeepCopy()
	}
	return nil
}

func (p *applier) Proxy() error {
	engine := p.DBEngine
	pg := p.PerconaPGCluster
	database := p.DB

	pgbouncerAvailVersions, ok := engine.Status.AvailableVersions.Proxy["pgbouncer"]
	if !ok {
		return errors.New("pgbouncer version not available")
	}
	pgbouncerVersion, ok := pgbouncerAvailVersions[database.Spec.Engine.Version]
	if !ok {
		return fmt.Errorf("pgbouncer version %s not available", database.Spec.Engine.Version)
	}
	pg.Spec.Proxy.PGBouncer.Image = pgbouncerVersion.ImagePath

	if database.Spec.Proxy.Replicas == nil {
		// By default we set the same number of replicas as the engine
		pg.Spec.Proxy.PGBouncer.Replicas = &database.Spec.Engine.Replicas
	} else {
		pg.Spec.Proxy.PGBouncer.Replicas = database.Spec.Proxy.Replicas
	}
	switch database.Spec.Proxy.Expose.Type {
	case everestv1alpha1.ExposeTypeInternal:
		pg.Spec.Proxy.PGBouncer.ServiceExpose = &pgv2.ServiceExpose{
			Type: string(corev1.ServiceTypeClusterIP),
		}
	case everestv1alpha1.ExposeTypeExternal:
		pg.Spec.Proxy.PGBouncer.ServiceExpose = &pgv2.ServiceExpose{
			Type:                     string(corev1.ServiceTypeLoadBalancer),
			LoadBalancerSourceRanges: p.DB.Spec.Proxy.Expose.IPSourceRangesStringArray(),
		}
	default:
		return fmt.Errorf("invalid expose type %s", database.Spec.Proxy.Expose.Type)
	}

	if !database.Spec.Proxy.Resources.CPU.IsZero() {
		pg.Spec.Proxy.PGBouncer.Resources.Limits[corev1.ResourceCPU] = database.Spec.Proxy.Resources.CPU
	}
	if !database.Spec.Proxy.Resources.Memory.IsZero() {
		pg.Spec.Proxy.PGBouncer.Resources.Limits[corev1.ResourceMemory] = database.Spec.Proxy.Resources.Memory
	}
	pg.Spec.Proxy.PGBouncer.ExposeSuperusers = true
	pg.Spec.Users = []crunchyv1beta1.PostgresUserSpec{
		{
			Name:       "postgres",
			SecretName: crunchyv1beta1.PostgresIdentifier(database.Spec.Engine.UserSecretsName),
		},
	}
	if p.clusterType == common.ClusterTypeEKS {
		pg.Spec.Proxy.PGBouncer.Affinity = hostnameAffinity.DeepCopy()
	}
	return nil
}

func (p *applier) Backup() error {
	spec, err := p.reconcilePGBackupsSpec()
	if err != nil {
		return err
	}
	p.PerconaPGCluster.Spec.Backups = spec
	return nil
}

func (p *applier) DataSource() error {
	if p.DB.Spec.DataSource == nil {
		p.PerconaPGCluster.Spec.DataSource = nil
		return nil
	}

	// If the Database is ready, we will remove the DataSource.
	// This will prevent the restore from happening again.
	if p.DB.Status.Status == everestv1alpha1.AppStateReady {
		p.DB.Spec.DataSource = nil
		p.PerconaPGCluster.Spec.DataSource = nil
		return nil
	}

	spec, err := p.genPGDataSourceSpec()
	if err != nil {
		return err
	}
	p.PerconaPGCluster.Spec.DataSource = spec
	return nil
}

func (p *applier) Monitoring() error {
	monitoring, err := common.GetDBMonitoringConfig(p.ctx, p.C, p.MonitoringNs, p.DB)
	if err != nil {
		return err
	}
	p.PerconaPGCluster.Spec.PMM = defaultSpec(p.DB).PMM
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
	pg := p.PerconaPGCluster
	database := p.DB
	c := p.C
	ctx := p.ctx

	pg.Spec.PMM.Enabled = true
	image := common.DefaultPMMClientImage
	if monitoring.Spec.PMM.Image != "" {
		image = monitoring.Spec.PMM.Image
	}
	pmmURL, err := url.Parse(monitoring.Spec.PMM.URL)
	if err != nil {
		return errors.Join(err, errors.New("invalid monitoring URL"))
	}
	pg.Spec.PMM.ServerHost = pmmURL.Hostname()
	pg.Spec.PMM.Image = image
	pg.Spec.PMM.Resources = database.Spec.Monitoring.Resources

	apiKey, err := common.GetSecretFromMonitoringConfig(ctx, c, monitoring, p.MonitoringNs)
	if err != nil {
		return err
	}

	if err := common.CreateOrUpdateSecretData(ctx, c, database, pg.Spec.PMM.Secret,
		map[string][]byte{
			"PMM_SERVER_KEY": []byte(apiKey),
		},
	); err != nil {
		return err
	}
	return nil
}

func defaultSpec(db *everestv1alpha1.DatabaseCluster) pgv2.PerconaPGClusterSpec {
	return pgv2.PerconaPGClusterSpec{
		InstanceSets: pgv2.PGInstanceSets{
			{
				Name: "instance1",
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{},
				},
			},
		},
		PMM: &pgv2.PMMSpec{
			Enabled: false,
			Resources: corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("300M"),
					corev1.ResourceCPU:    resource.MustParse("500m"),
				},
			},
			Secret: fmt.Sprintf("%s%s-pmm", common.EverestSecretsPrefix, db.GetName()),
		},
		Proxy: &pgv2.PGProxySpec{
			PGBouncer: &pgv2.PGBouncerSpec{
				Resources: corev1.ResourceRequirements{
					// XXX: Remove this once templates will be available
					Limits: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("128Mi"),
						corev1.ResourceCPU:    resource.MustParse("200m"),
					},
				},
			},
		},
	}
}

func (p *applier) updatePGConfig(
	pg *pgv2.PerconaPGCluster, db *everestv1alpha1.DatabaseCluster,
) error {
	//nolint:godox
	// TODO: uncomment once https://perconadev.atlassian.net/browse/K8SPG-518 done if db.Spec.Engine.Config == "" {
	//	 if pg.Spec.Patroni == nil {
	//		 return nil
	//	 }
	//	 pg.Spec.Patroni.DynamicConfiguration = nil
	//	 return nil
	// }

	parser := NewConfigParser(db.Spec.Engine.Config)
	cfg, err := parser.ParsePGConfig()
	if err != nil {
		return err
	}

	//nolint:godox
	// TODO: remove once https://perconadev.atlassian.net/browse/K8SPG-518 done
	cfg["track_commit_timestamp"] = "on"

	if len(cfg) == 0 {
		return nil
	}

	if pg.Spec.Patroni == nil {
		pg.Spec.Patroni = &crunchyv1beta1.PatroniSpec{}
	}

	if pg.Spec.Patroni.DynamicConfiguration == nil {
		pg.Spec.Patroni.DynamicConfiguration = make(crunchyv1beta1.SchemalessObject)
	}

	dc := pg.Spec.Patroni.DynamicConfiguration
	if _, ok := dc["postgresql"]; !ok {
		dc["postgresql"] = make(map[string]any)
	}

	dcPG, ok := dc["postgresql"].(map[string]any)
	if !ok {
		return errors.New("could not assert postgresql as map[string]any")
	}

	if _, ok := dcPG["parameters"]; !ok {
		dcPG["parameters"] = make(map[string]any)
	}

	if _, ok := dcPG["parameters"].(map[string]any); !ok {
		return errors.New("could not assert postgresql.parameters as map[string]any")
	}

	dcPG["parameters"] = cfg

	return nil
}

// getBackupInfo returns the backup base name, backup storage name, and destination of the backup.
func getBackupInfo(
	ctx context.Context,
	c client.Client,
	database *everestv1alpha1.DatabaseCluster) (
	string, string, string, error,
) {
	backupBaseName := ""
	backupStorageName := ""
	dest := ""
	if database.Spec.DataSource.DBClusterBackupName != "" {
		dbClusterBackup := &everestv1alpha1.DatabaseClusterBackup{}
		err := c.Get(ctx, types.NamespacedName{Name: database.Spec.DataSource.DBClusterBackupName, Namespace: database.Namespace}, dbClusterBackup)
		if err != nil {
			return "", "", "", errors.Join(err, fmt.Errorf("failed to get DBClusterBackup %s", database.Spec.DataSource.DBClusterBackupName))
		}

		if dbClusterBackup.Status.Destination == nil {
			return "", "", "", fmt.Errorf("DBClusterBackup %s has no destination", database.Spec.DataSource.DBClusterBackupName)
		}

		backupBaseName = filepath.Base(*dbClusterBackup.Status.Destination)
		backupStorageName = dbClusterBackup.Spec.BackupStorageName
		dest = *dbClusterBackup.Status.Destination
	}

	if database.Spec.DataSource.BackupSource != nil {
		backupBaseName = filepath.Base(database.Spec.DataSource.BackupSource.Path)
		backupStorageName = database.Spec.DataSource.BackupSource.BackupStorageName
	}
	return backupBaseName, backupStorageName, dest, nil
}

// handlePGDataSourceAzure handles the PGDataSource object for S3 backup storage.
func (p *applier) handlePGDataSourceAzure(
	repoName string,
	pgDataSource *crunchyv1beta1.DataSource,
	backupStorage *everestv1alpha1.BackupStorage,
	database *everestv1alpha1.DatabaseCluster,
) error {
	c := p.C
	ctx := p.ctx
	pgBackRestSecretIni, err := ini.Load([]byte{})
	if err != nil {
		return errors.Join(err, errors.New("failed to initialize PGBackrest secret data"))
	}

	backupStorageSecret := &corev1.Secret{}
	err = c.Get(ctx, types.NamespacedName{Name: backupStorage.Spec.CredentialsSecretName, Namespace: backupStorage.Namespace}, backupStorageSecret)
	if err != nil {
		return errors.Join(err, fmt.Errorf("failed to get backup storage secret %s", backupStorage.Spec.CredentialsSecretName))
	}

	err = addBackupStorageCredentialsToPGBackrestSecretIni(
		everestv1alpha1.BackupStorageTypeAzure, pgBackRestSecretIni, repoName, backupStorageSecret,
	)
	if err != nil {
		return errors.Join(err, errors.New("failed to add data source storage credentials to PGBackrest secret data"))
	}
	pgBackrestSecretBuf := new(bytes.Buffer)
	if _, err = pgBackRestSecretIni.WriteTo(pgBackrestSecretBuf); err != nil {
		return errors.Join(err, errors.New("failed to write PGBackrest secret data"))
	}

	secretData := pgBackrestSecretBuf.Bytes()

	pgBackrestSecret, err := p.createPGBackrestSecret(
		"azure.conf",
		secretData,
		database.GetName()+"-pgbackrest-datasource-secrets",
		nil,
	)
	if err != nil {
		return errors.Join(err, errors.New("failed to create pgbackrest secret"))
	}

	pgDataSource.PGBackRest.Configuration = []corev1.VolumeProjection{
		{
			Secret: &corev1.SecretProjection{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: pgBackrestSecret.Name,
				},
			},
		},
	}
	pgDataSource.PGBackRest.Repo = crunchyv1beta1.PGBackRestRepo{
		Name: repoName,
		Azure: &crunchyv1beta1.RepoAzure{
			Container: backupStorage.Spec.Bucket,
		},
	}
	return nil
}

// handlePGDataSourceS3 handles the PGDataSource object for S3 backup storage.
func (p *applier) handlePGDataSourceS3(
	repoName string,
	pgDataSource *crunchyv1beta1.DataSource,
	backupStorage *everestv1alpha1.BackupStorage,
	database *everestv1alpha1.DatabaseCluster,
) error {
	c := p.C
	ctx := p.ctx
	pgBackRestSecretIni, err := ini.Load([]byte{})
	if err != nil {
		return errors.Join(err, errors.New("failed to initialize PGBackrest secret data"))
	}

	backupStorageSecret := &corev1.Secret{}
	err = c.Get(ctx, types.NamespacedName{Name: backupStorage.Spec.CredentialsSecretName, Namespace: backupStorage.Namespace}, backupStorageSecret)
	if err != nil {
		return errors.Join(err, fmt.Errorf("failed to get backup storage secret %s", backupStorage.Spec.CredentialsSecretName))
	}

	err = addBackupStorageCredentialsToPGBackrestSecretIni(everestv1alpha1.BackupStorageTypeS3, pgBackRestSecretIni, repoName, backupStorageSecret)
	if err != nil {
		return errors.Join(err, errors.New("failed to add data source storage credentials to PGBackrest secret data"))
	}
	pgBackrestSecretBuf := new(bytes.Buffer)
	if _, err = pgBackRestSecretIni.WriteTo(pgBackrestSecretBuf); err != nil {
		return errors.Join(err, errors.New("failed to write PGBackrest secret data"))
	}

	secretData := pgBackrestSecretBuf.Bytes()

	pgBackrestSecret, err := p.createPGBackrestSecret(
		"s3.conf",
		secretData,
		database.GetName()+"-pgbackrest-datasource-secrets",
		nil,
	)
	if err != nil {
		return errors.Join(err, errors.New("failed to create pgbackrest secret"))
	}

	pgDataSource.PGBackRest.Configuration = []corev1.VolumeProjection{
		{
			Secret: &corev1.SecretProjection{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: pgBackrestSecret.Name,
				},
			},
		},
	}
	pgDataSource.PGBackRest.Repo = crunchyv1beta1.PGBackRestRepo{
		Name: repoName,
		S3: &crunchyv1beta1.RepoS3{
			Bucket:   backupStorage.Spec.Bucket,
			Endpoint: backupStorage.Spec.EndpointURL,
			Region:   backupStorage.Spec.Region,
		},
	}
	return nil
}

func (p *applier) genPGDataSourceSpec() (*crunchyv1beta1.DataSource, error) {
	database := p.DB
	ctx := p.ctx
	c := p.C
	if (database.Spec.DataSource.DBClusterBackupName == "" &&
		database.Spec.DataSource.BackupSource == nil) ||
		(database.Spec.DataSource.DBClusterBackupName != "" &&
			database.Spec.DataSource.BackupSource != nil) {
		return nil,
			errors.New("either DBClusterBackupName or BackupSource must be specified in the DataSource field")
	}

	backupBaseName, backupStorageName, dest, err := getBackupInfo(ctx, c, database)
	if err != nil {
		return nil, err
	}

	backupStorage, err := common.GetBackupStorage(ctx, c, backupStorageName, p.SystemNs)
	if err != nil {
		return nil, err
	}

	if database.GetNamespace() != p.SystemNs {
		if err := common.ReconcileBackupStorageSecret(ctx, c, p.SystemNs, backupStorage, database); err != nil {
			return nil, err
		}
	}

	repoName := "repo1"
	options, err := getPGRestoreOptions(*database.Spec.DataSource, backupStorage, backupBaseName, repoName)
	if err != nil {
		return nil, err
	}

	// Initialize a PG datasource object.
	pgDataSource := &crunchyv1beta1.DataSource{
		PGBackRest: &crunchyv1beta1.PGBackRestDataSource{
			Global: map[string]string{
				fmt.Sprintf(pgBackRestPathTmpl, repoName): globalDatasourceDestination(dest, database, backupStorage),
			},
			Stanza:  "db",
			Options: options,
			// XXX: Remove this once templates will be available
			Resources: corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("128Mi"),
					corev1.ResourceCPU:    resource.MustParse("200m"),
				},
			},
		},
	}

	// Handle PG Datasource based on the backup storage type.
	switch backupStorage.Spec.Type {
	case everestv1alpha1.BackupStorageTypeS3:
		if err := p.handlePGDataSourceS3(repoName, pgDataSource, backupStorage, database); err != nil {
			return nil, err
		}
	case everestv1alpha1.BackupStorageTypeAzure:
		if err := p.handlePGDataSourceAzure(repoName, pgDataSource, backupStorage, database); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("unsupported backup storage type %s for %s", backupStorage.Spec.Type, backupStorage.Name)
	}
	return pgDataSource, nil
}

func getPGRestoreOptions(
	dataSource everestv1alpha1.DataSource,
	backupStorage *everestv1alpha1.BackupStorage,
	backupBaseName, repoName string,
) ([]string, error) {
	options := []string{
		"--set=" + backupBaseName,
	}

	if pointer.Get(backupStorage.Spec.ForcePathStyle) {
		options = append(options, "--"+fmt.Sprintf(pgBackRestStorageForcePathTmpl, repoName)+"="+pgBackRestStoragePathStyle)
	}
	if !pointer.Get(backupStorage.Spec.VerifyTLS) {
		options = append(options, "--no-"+fmt.Sprintf(pgBackRestStorageVerifyTmpl, repoName))
	}

	if dataSource.PITR != nil {
		if err := common.ValidatePitrRestoreSpec(dataSource); err != nil {
			return nil, err
		}
		dateString := fmt.Sprintf(`"%s"`, dataSource.PITR.Date.Format(everestv1alpha1.DateFormatSpace))
		options = append(options, "--type="+pgBackupTypeDate)
		options = append(options, "--target="+dateString)
	} else {
		options = append(options, "--type="+pgBackupTypeImmediate)
	}

	return options, nil
}

func globalDatasourceDestination(dest string, db *everestv1alpha1.DatabaseCluster, backupStorage *everestv1alpha1.BackupStorage) string {
	if dest == "" {
		dest = "/" + common.BackupStoragePrefix(db)
	} else {
		// Extract the relevant prefix from the backup destination
		switch backupStorage.Spec.Type {
		case everestv1alpha1.BackupStorageTypeS3:
			dest = strings.TrimPrefix(dest, "s3://")
		case everestv1alpha1.BackupStorageTypeAzure:
			dest = strings.TrimPrefix(dest, "azure://")
		}

		dest = strings.TrimPrefix(dest, backupStorage.Spec.Bucket)
		dest = strings.TrimLeft(dest, "/")
		prefix := common.BackupStoragePrefix(db)
		prefixCount := len(strings.Split(prefix, "/"))
		dest = "/" + strings.Join(strings.SplitN(dest, "/", prefixCount+1)[0:prefixCount], "/")
	}
	return dest
}

// Adds the backup storage credentials to the PGBackrest secret data.
func addBackupStorageCredentialsToPGBackrestSecretIni(
	storageType everestv1alpha1.BackupStorageType,
	cfg *ini.File,
	repoName string,
	secret *corev1.Secret,
) error {
	switch storageType {
	case everestv1alpha1.BackupStorageTypeS3:
		_, err := cfg.Section("global").NewKey(
			repoName+"-s3-key",
			string(secret.Data["AWS_ACCESS_KEY_ID"]),
		)
		if err != nil {
			return err
		}

		_, err = cfg.Section("global").NewKey(
			repoName+"-s3-key-secret",
			string(secret.Data["AWS_SECRET_ACCESS_KEY"]),
		)
		if err != nil {
			return err
		}
	case everestv1alpha1.BackupStorageTypeAzure:
		_, err := cfg.Section("global").NewKey(
			repoName+"-azure-account",
			string(secret.Data["AZURE_STORAGE_ACCOUNT_NAME"]),
		)
		if err != nil {
			return err
		}

		_, err = cfg.Section("global").NewKey(
			repoName+"-azure-key",
			string(secret.Data["AZURE_STORAGE_ACCOUNT_KEY"]),
		)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("backup storage type %s not supported", storageType)
	}

	return nil
}

// createPGBackrestSecret creates or updates the PG Backrest secret.
func (p *applier) createPGBackrestSecret(
	pgbackrestKey string,
	pgbackrestConf []byte,
	pgBackrestSecretName string,
	additionalSecrets map[string][]byte,
) (*corev1.Secret, error) {
	ctx := p.ctx
	database := p.DB
	c := p.C
	pgBackrestSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pgBackrestSecretName,
			Namespace: database.Namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			pgbackrestKey: pgbackrestConf,
		},
	}

	for k, v := range additionalSecrets {
		pgBackrestSecret.Data[k] = v
	}

	err := controllerutil.SetControllerReference(database, pgBackrestSecret, c.Scheme())
	if err != nil {
		return nil, err
	}
	err = common.CreateOrUpdate(ctx, c, pgBackrestSecret, false)
	if err != nil {
		return nil, err
	}

	return pgBackrestSecret, nil
}

// Add backup storages used by restores to the list.
func (p *applier) addBackupStoragesByRestores(
	backupList *everestv1alpha1.DatabaseClusterBackupList,
	restoreList *everestv1alpha1.DatabaseClusterRestoreList,
	backupStorages map[string]everestv1alpha1.BackupStorageSpec,
	backupStoragesSecrets map[string]*corev1.Secret,
) error {
	ctx := p.ctx
	database := p.DB
	c := p.C
	for _, restore := range restoreList.Items {
		// If the restore has already completed, skip it.
		if restore.IsComplete(database.Spec.Engine.Type) {
			continue
		}

		var backupStorageName string
		if restore.Spec.DataSource.DBClusterBackupName != "" {
			backup := &everestv1alpha1.DatabaseClusterBackup{}
			key := types.NamespacedName{
				Name:      restore.Spec.DataSource.DBClusterBackupName,
				Namespace: restore.GetNamespace(),
			}
			err := c.Get(ctx, key, backup)
			if err != nil {
				return errors.Join(err, errors.New("failed to get DatabaseClusterBackup"))
			}
			backupStorageName = backup.Spec.BackupStorageName
		}
		if restore.Spec.DataSource.BackupSource != nil {
			backupStorageName = restore.Spec.DataSource.BackupSource.BackupStorageName
		}

		// Check if we already fetched that backup storage
		if _, ok := backupStorages[backupStorageName]; ok {
			continue
		}

		backupStorage, err := common.GetBackupStorage(ctx, c, backupStorageName, p.SystemNs)
		if err != nil {
			return err
		}
		if database.GetNamespace() != p.SystemNs {
			if err := common.ReconcileBackupStorageSecret(ctx, c, p.SystemNs, backupStorage, database); err != nil {
				return err
			}
		}

		// Get the Secret used by the BackupStorage.
		backupStorageSecret := &corev1.Secret{}
		key := types.NamespacedName{Name: backupStorage.Spec.CredentialsSecretName, Namespace: backupStorage.GetNamespace()}
		err = c.Get(ctx, key, backupStorageSecret)
		if err != nil {
			return errors.Join(err,
				fmt.Errorf("failed to get backup storage secret %s", backupStorage.Spec.CredentialsSecretName))
		}

		backupStorages[backupStorage.Name] = backupStorage.Spec
		backupStoragesSecrets[backupStorage.Name] = backupStorageSecret

		// XXX We need to add this restore's backup storage to the list of
		// repos.
		// Passing the restoreList to the reconcilePGBackRestRepos function is
		// not a good option because in that case the
		// reconcilePGBackRestRepos function would no longer be purely
		// functional, as it would then need to fetch the DatabaseClusterBackup
		// object referenced by the restore from the API server.
		// We could instead create a new restore type that would also include
		// the backup storage and fetch it here, but that seems a bit overkill.
		// We already pass on the backupList to the reconcilePGBackRestRepos
		// function that only uses the DatabaseClusterBackup reference to the
		// backup storage name which is exactly what we need here too. Let's
		// just add a fake backup to the backupList so that the restore's
		// backup storage is included in the repos.
		backupList.Items = append(backupList.Items, everestv1alpha1.DatabaseClusterBackup{
			Spec: everestv1alpha1.DatabaseClusterBackupSpec{
				BackupStorageName: backupStorageName,
			},
		})
	}
	return nil
}

// Add backup storages used by backup schedules to the list.
func (p *applier) addBackupStoragesBySchedules(
	backupSchedules []everestv1alpha1.BackupSchedule,
	backupStorages map[string]everestv1alpha1.BackupStorageSpec,
	backupStoragesSecrets map[string]*corev1.Secret,
) error {
	ctx := p.ctx
	database := p.DB
	c := p.C
	// Add backup storages used by backup schedules to the list
	for _, schedule := range backupSchedules {
		// Check if we already fetched that backup storage
		if _, ok := backupStorages[schedule.BackupStorageName]; ok {
			continue
		}

		backupStorage, err := common.GetBackupStorage(ctx, c, schedule.BackupStorageName, p.SystemNs)
		if err != nil {
			return err
		}
		if database.GetNamespace() != p.SystemNs {
			if err := common.ReconcileBackupStorageSecret(ctx, c, p.SystemNs, backupStorage, database); err != nil {
				return err
			}
		}

		backupStorageSecret := &corev1.Secret{}
		err = c.Get(ctx, types.NamespacedName{Name: backupStorage.Spec.CredentialsSecretName, Namespace: backupStorage.Namespace}, backupStorageSecret)
		if err != nil {
			return errors.Join(err, fmt.Errorf("failed to get backup storage secret %s", backupStorage.Spec.CredentialsSecretName))
		}

		backupStorages[backupStorage.Name] = backupStorage.Spec
		backupStoragesSecrets[backupStorage.Name] = backupStorageSecret
	}
	return nil
}

// Add backup storages used by on-demand backups to the list.
func (p *applier) addBackupStoragesByOnDemandBackups(
	backupList *everestv1alpha1.DatabaseClusterBackupList,
	backupStorages map[string]everestv1alpha1.BackupStorageSpec,
	backupStoragesSecrets map[string]*corev1.Secret,
) error {
	ctx := p.ctx
	database := p.DB
	c := p.C
	for _, backup := range backupList.Items {
		// Check if we already fetched that backup storage
		if _, ok := backupStorages[backup.Spec.BackupStorageName]; ok {
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

		backupStorageSecret := &corev1.Secret{}
		err = c.Get(ctx, types.NamespacedName{Name: backupStorage.Spec.CredentialsSecretName, Namespace: backupStorage.Namespace}, backupStorageSecret)
		if err != nil {
			return fmt.Errorf("failed to get backup storage secret '%s': %w", backupStorage.Spec.CredentialsSecretName, err)
		}

		backupStorages[backupStorage.Name] = backupStorage.Spec
		backupStoragesSecrets[backupStorage.Name] = backupStorageSecret
	}
	return nil
}

func (p *applier) reconcilePGBackupsSpec() (crunchyv1beta1.Backups, error) {
	ctx := p.ctx
	c := p.C
	database := p.DB
	engine := p.DBEngine
	oldBackups := p.PerconaPGCluster.Spec.Backups

	pgbackrestVersion, ok := engine.Status.AvailableVersions.Backup[database.Spec.Engine.Version]
	if !ok {
		return crunchyv1beta1.Backups{}, fmt.Errorf("pgbackrest version %s not available", database.Spec.Engine.Version)
	}

	newBackups := crunchyv1beta1.Backups{
		PGBackRest: crunchyv1beta1.PGBackRestArchive{
			Image:   pgbackrestVersion.ImagePath,
			Manual:  oldBackups.PGBackRest.Manual,
			Restore: oldBackups.PGBackRest.Restore,
		},
	}

	if newBackups.PGBackRest.Manual == nil {
		// This field is required by the operator, but it doesn't impact
		// our manual backup operation because we use the PerconaPGBackup
		// CR to request on-demand backups which then changes the Manual field
		// internally and sets the
		// postgres-operator.crunchydata.com/pgbackrest-backup annotation.
		newBackups.PGBackRest.Manual = &crunchyv1beta1.PGBackRestManualBackup{
			RepoName: "repo1",
		}
	}

	// List DatabaseClusterBackup objects for this database
	backupList, err := common.ListDatabaseClusterBackups(ctx, c, database.GetName(), database.GetNamespace())
	if err != nil {
		return crunchyv1beta1.Backups{}, err
	}
	// Remove the init backup.
	backupList.Items = slices.DeleteFunc(backupList.Items, func(backup everestv1alpha1.DatabaseClusterBackup) bool {
		return backup.Spec.BackupStorageName == everestv1alpha1.LocalBackupStorageName(database.GetName())
	})

	backupStorages := map[string]everestv1alpha1.BackupStorageSpec{}
	backupStoragesSecrets := map[string]*corev1.Secret{}

	// Add backup storages used by on-demand backups to the list
	if err := p.addBackupStoragesByOnDemandBackups(backupList, backupStorages, backupStoragesSecrets); err != nil {
		return crunchyv1beta1.Backups{}, err
	}

	// List DatabaseClusterRestore objects for this database
	restoreList, err := common.ListDatabaseClusterRestores(ctx, c, database.GetName(), database.GetNamespace())
	if err != nil {
		return crunchyv1beta1.Backups{}, err
	}

	// Add backup storages used by restores to the list
	if err := p.addBackupStoragesByRestores(backupList, restoreList, backupStorages, backupStoragesSecrets); err != nil {
		return crunchyv1beta1.Backups{}, err
	}

	// Only use the backup schedules if schedules are enabled in the DBC spec
	backupSchedules := []everestv1alpha1.BackupSchedule{}
	if database.Spec.Backup.Enabled {
		backupSchedules = database.Spec.Backup.Schedules
	}
	// Add backup storages used by backup schedules to the list
	if err := p.addBackupStoragesBySchedules(backupSchedules, backupStorages, backupStoragesSecrets); err != nil {
		return crunchyv1beta1.Backups{}, err
	}

	pgInitLocalBackupStorage, err := common.GetBackupStorage(ctx, c, everestv1alpha1.LocalBackupStorageName(p.DB.GetName()), p.SystemNs)
	if err != nil {
		return crunchyv1beta1.Backups{},
			fmt.Errorf("cannot get initial local backup storage for Postgres: %w", err)
	}

	pgBackrestRepos, pgBackrestGlobal, pgBackrestSecretData, err := reconcilePGBackRestRepos(
		oldBackups.PGBackRest.Repos,
		backupSchedules,
		backupList.Items,
		backupStorages,
		backupStoragesSecrets,
		database,
		*pgInitLocalBackupStorage.Spec.PVCSpec, // Creation of this BackupStorage is handled by us, so we can guarantee that it won't be nil.
	)
	if err != nil {
		return crunchyv1beta1.Backups{}, err
	}

	pgBackrestSecret, err := createPGBackrestSecret(
		ctx,
		c,
		database,
		"s3.conf",
		pgBackrestSecretData,
		database.Name+"-pgbackrest-secrets",
		nil,
	)
	if err != nil {
		return crunchyv1beta1.Backups{}, err
	}

	newBackups.PGBackRest.Configuration = []corev1.VolumeProjection{
		{
			Secret: &corev1.SecretProjection{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: pgBackrestSecret.Name,
				},
			},
		},
	}

	newBackups.PGBackRest.Repos = pgBackrestRepos
	newBackups.PGBackRest.Global = pgBackrestGlobal
	return newBackups, nil
}

// createPGBackrestSecret creates or updates the PG Backrest secret.
func createPGBackrestSecret(
	ctx context.Context,
	c client.Client,
	database *everestv1alpha1.DatabaseCluster,
	pgbackrestKey string,
	pgbackrestConf []byte,
	pgBackrestSecretName string,
	additionalSecrets map[string][]byte,
) (*corev1.Secret, error) {
	pgBackrestSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pgBackrestSecretName,
			Namespace: database.Namespace,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			pgbackrestKey: pgbackrestConf,
		},
	}

	for k, v := range additionalSecrets {
		pgBackrestSecret.Data[k] = v
	}

	err := controllerutil.SetControllerReference(database, pgBackrestSecret, c.Scheme())
	if err != nil {
		return nil, err
	}
	err = common.CreateOrUpdate(ctx, c, pgBackrestSecret, false)
	if err != nil {
		return nil, err
	}

	return pgBackrestSecret, nil
}

// TODO: needs refactor
//
//nolint:gocognit,gocyclo,cyclop,maintidx, godox
func reconcilePGBackRestRepos(
	oldRepos []crunchyv1beta1.PGBackRestRepo,
	backupSchedules []everestv1alpha1.BackupSchedule,
	backupRequests []everestv1alpha1.DatabaseClusterBackup,
	backupStorages map[string]everestv1alpha1.BackupStorageSpec,
	backupStoragesSecrets map[string]*corev1.Secret,
	db *everestv1alpha1.DatabaseCluster,
	pgInitPVCSpec corev1.PersistentVolumeClaimSpec,
) (
	[]crunchyv1beta1.PGBackRestRepo,
	map[string]string,
	[]byte,
	error,
) {
	backupStoragesInRepos := map[string]struct{}{}
	pgBackRestGlobal := map[string]string{}
	pgBackRestSecretIni, err := ini.Load([]byte{})
	if err != nil {
		return []crunchyv1beta1.PGBackRestRepo{},
			map[string]string{},
			[]byte{},
			errors.Join(err, errors.New("failed to initialize PGBackrest secret data"))
	}

	availableRepoNames := []string{
		// repo1 is reserved for the PVC-based repo, see below for details on
		// how it is handled
		"repo2",
		"repo3",
		"repo4",
	}

	reposReconciled := list.New()
	reposToBeReconciled := list.New()
	for _, repo := range oldRepos {
		reposToBeReconciled.PushBack(repo)
	}
	backupSchedulesReconciled := list.New()
	backupSchedulesToBeReconciled := list.New()
	for _, backupSchedule := range backupSchedules {
		if !backupSchedule.Enabled {
			continue
		}
		backupSchedulesToBeReconciled.PushBack(backupSchedule)
	}

	// Move repos with set schedules which are already correct to the
	// reconciled list
	var ebNext *list.Element
	var erNext *list.Element
	for eb := backupSchedulesToBeReconciled.Front(); eb != nil; eb = ebNext {
		// Save the next element because we might remove the current one
		ebNext = eb.Next()

		backupSchedule, ok := eb.Value.(everestv1alpha1.BackupSchedule)
		if !ok {
			return []crunchyv1beta1.PGBackRestRepo{},
				map[string]string{},
				[]byte{},
				fmt.Errorf("failed to cast backup schedule %v", eb.Value)
		}
		for er := reposToBeReconciled.Front(); er != nil; er = erNext {
			// Save the next element because we might remove the current one
			erNext = er.Next()

			repo, ok := er.Value.(crunchyv1beta1.PGBackRestRepo)
			if !ok {
				return []crunchyv1beta1.PGBackRestRepo{},
					map[string]string{},
					[]byte{},
					fmt.Errorf("failed to cast repo %v", er.Value)
			}
			repoBackupStorageName := backupStorageNameFromRepo(backupStorages, repo)
			if backupSchedule.BackupStorageName != repoBackupStorageName ||
				repo.BackupSchedules == nil ||
				repo.BackupSchedules.Full == nil ||
				backupSchedule.Schedule != *repo.BackupSchedules.Full {
				continue
			}

			reposReconciled.PushBack(repo)
			reposToBeReconciled.Remove(er)
			backupSchedulesReconciled.PushBack(backupSchedule)
			backupSchedulesToBeReconciled.Remove(eb)
			availableRepoNames = removeRepoName(availableRepoNames, repo.Name)

			// Keep track of backup storages which are already in use by a repo
			backupStoragesInRepos[backupSchedule.BackupStorageName] = struct{}{}

			pgBackRestGlobal[fmt.Sprintf(pgBackRestPathTmpl, repo.Name)] = "/" + common.BackupStoragePrefix(db)
			pgBackRestGlobal[fmt.Sprintf(pgBackRestRetentionTmpl, repo.Name)] = strconv.Itoa(getPGRetentionCopies(backupSchedule.RetentionCopies))
			sType, err := backupStorageTypeFromBackrestRepo(repo)
			if err != nil {
				return []crunchyv1beta1.PGBackRestRepo{},
					map[string]string{},
					[]byte{},
					err
			}
			if verify := pointer.Get(backupStorages[repo.Name].VerifyTLS); !verify {
				// See: https://pgbackrest.org/configuration.html#section-repository/option-repo-storage-verify-tls
				pgBackRestGlobal[fmt.Sprintf(pgBackRestStorageVerifyTmpl, repo.Name)] = "n"
			}
			if forcePathStyle := pointer.Get(backupStorages[repo.Name].ForcePathStyle); !forcePathStyle {
				// See: https://pgbackrest.org/configuration.html#section-repository/option-repo-s3-uri-style
				pgBackRestGlobal[fmt.Sprintf(pgBackRestStorageForcePathTmpl, repo.Name)] = pgBackRestStoragePathStyle
			}

			err = addBackupStorageCredentialsToPGBackrestSecretIni(
				sType,
				pgBackRestSecretIni,
				repo.Name,
				backupStoragesSecrets[repoBackupStorageName])
			if err != nil {
				return []crunchyv1beta1.PGBackRestRepo{},
					map[string]string{},
					[]byte{},
					fmt.Errorf("failed to add backup storage credentials to PGBackrest secret data: %w", err)
			}

			break
		}
	}

	// Update the schedules of the repos which already exist but have the wrong
	// schedule and move them to the reconciled list
	for er := reposToBeReconciled.Front(); er != nil; er = erNext {
		// Save the next element because we might remove the current one
		erNext = er.Next()

		repo, ok := er.Value.(crunchyv1beta1.PGBackRestRepo)
		if !ok {
			return []crunchyv1beta1.PGBackRestRepo{},
				map[string]string{},
				[]byte{},
				fmt.Errorf("failed to cast repo %v", er.Value)
		}
		repoBackupStorageName := backupStorageNameFromRepo(backupStorages, repo)
		for eb := backupSchedulesToBeReconciled.Front(); eb != nil; eb = ebNext {
			// Save the next element because we might remove the current one
			ebNext = eb.Next()

			backupSchedule, ok := eb.Value.(everestv1alpha1.BackupSchedule)
			if !ok {
				return []crunchyv1beta1.PGBackRestRepo{},
					map[string]string{},
					[]byte{},
					fmt.Errorf("failed to cast backup schedule %v", eb.Value)
			}
			if backupSchedule.BackupStorageName == repoBackupStorageName {
				repo.BackupSchedules = &crunchyv1beta1.PGBackRestBackupSchedules{
					Full: &backupSchedule.Schedule,
				}

				reposReconciled.PushBack(repo)
				reposToBeReconciled.Remove(er)
				backupSchedulesReconciled.PushBack(backupSchedule)
				backupSchedulesToBeReconciled.Remove(eb)
				availableRepoNames = removeRepoName(availableRepoNames, repo.Name)

				// Keep track of backup storages which are already in use by a repo
				backupStoragesInRepos[backupSchedule.BackupStorageName] = struct{}{}

				pgBackRestGlobal[fmt.Sprintf(pgBackRestPathTmpl, repo.Name)] = "/" + common.BackupStoragePrefix(db)
				pgBackRestGlobal[fmt.Sprintf(pgBackRestRetentionTmpl, repo.Name)] = strconv.Itoa(getPGRetentionCopies(backupSchedule.RetentionCopies))
				if verify := pointer.Get(backupStorages[repo.Name].VerifyTLS); !verify {
					// See: https://pgbackrest.org/configuration.html#section-repository/option-repo-storage-verify-tls
					pgBackRestGlobal[fmt.Sprintf(pgBackRestStorageVerifyTmpl, repo.Name)] = "n"
				}
				if forcePathStyle := pointer.Get(backupStorages[repo.Name].ForcePathStyle); !forcePathStyle {
					// See: https://pgbackrest.org/configuration.html#section-repository/option-repo-s3-uri-style
					pgBackRestGlobal[fmt.Sprintf(pgBackRestStorageForcePathTmpl, repo.Name)] = pgBackRestStoragePathStyle
				}

				sType, err := backupStorageTypeFromBackrestRepo(repo)
				if err != nil {
					return []crunchyv1beta1.PGBackRestRepo{},
						map[string]string{},
						[]byte{},
						err
				}

				err = addBackupStorageCredentialsToPGBackrestSecretIni(sType, pgBackRestSecretIni, repo.Name, backupStoragesSecrets[repoBackupStorageName])
				if err != nil {
					return []crunchyv1beta1.PGBackRestRepo{},
						map[string]string{},
						[]byte{},
						errors.Join(err, errors.New("failed to add backup storage credentials to PGBackrest secret data"))
				}

				break
			}
		}
	}

	// Add new backup schedules
	for eb := backupSchedulesToBeReconciled.Front(); eb != nil; eb = eb.Next() {
		backupSchedule, ok := eb.Value.(everestv1alpha1.BackupSchedule)
		if !ok {
			return []crunchyv1beta1.PGBackRestRepo{},
				map[string]string{},
				[]byte{},
				fmt.Errorf("failed to cast backup schedule %v", eb.Value)
		}
		backupStorage, ok := backupStorages[backupSchedule.BackupStorageName]
		if !ok {
			return []crunchyv1beta1.PGBackRestRepo{},
				map[string]string{},
				[]byte{},
				fmt.Errorf("unknown backup storage %s", backupSchedule.BackupStorageName)
		}

		if len(availableRepoNames) == 0 {
			return []crunchyv1beta1.PGBackRestRepo{},
				map[string]string{},
				[]byte{},
				errors.New("exceeded max number of repos")
		}

		repoName := availableRepoNames[0]
		availableRepoNames = removeRepoName(availableRepoNames, repoName)
		repo, err := genPGBackrestRepo(repoName, backupStorage, &backupSchedule.Schedule)
		if err != nil {
			return []crunchyv1beta1.PGBackRestRepo{}, map[string]string{}, []byte{}, err
		}
		reposReconciled.PushBack(repo)

		// Keep track of backup storages which are already in use by a repo
		backupStoragesInRepos[backupSchedule.BackupStorageName] = struct{}{}

		pgBackRestGlobal[fmt.Sprintf(pgBackRestPathTmpl, repo.Name)] = "/" + common.BackupStoragePrefix(db)
		pgBackRestGlobal[fmt.Sprintf(pgBackRestRetentionTmpl, repo.Name)] = strconv.Itoa(getPGRetentionCopies(backupSchedule.RetentionCopies))
		if verify := pointer.Get(backupStorage.VerifyTLS); !verify {
			// See: https://pgbackrest.org/configuration.html#section-repository/option-repo-storage-verify-tls
			pgBackRestGlobal[fmt.Sprintf(pgBackRestStorageVerifyTmpl, repo.Name)] = "n"
		}
		if forcePathStyle := pointer.Get(backupStorages[repo.Name].ForcePathStyle); !forcePathStyle {
			// See: https://pgbackrest.org/configuration.html#section-repository/option-repo-s3-uri-style
			pgBackRestGlobal[fmt.Sprintf(pgBackRestStorageForcePathTmpl, repo.Name)] = pgBackRestStoragePathStyle
		}
		sType, err := backupStorageTypeFromBackrestRepo(repo)
		if err != nil {
			return []crunchyv1beta1.PGBackRestRepo{},
				map[string]string{},
				[]byte{},
				err
		}

		err = addBackupStorageCredentialsToPGBackrestSecretIni(sType, pgBackRestSecretIni, repo.Name, backupStoragesSecrets[backupSchedule.BackupStorageName])
		if err != nil {
			return []crunchyv1beta1.PGBackRestRepo{},
				map[string]string{},
				[]byte{},
				errors.Join(err, errors.New("failed to add backup storage credentials to PGBackrest secret data"))
		}
	}

	// Add on-demand backups whose backup storage doesn't have a schedule
	// defined
	// XXX some of these backups might be fake, see the XXX comment in the
	// reconcilePGBackupsSpec function for more context. Be very careful if you
	// decide to use fields other that the Spec.BackupStorageName.
	for _, backupRequest := range backupRequests {
		// Backup storage already in repos, skip
		if _, ok := backupStoragesInRepos[backupRequest.Spec.BackupStorageName]; ok {
			continue
		}

		backupStorage, ok := backupStorages[backupRequest.Spec.BackupStorageName]
		if !ok {
			return []crunchyv1beta1.PGBackRestRepo{}, map[string]string{}, []byte{}, fmt.Errorf("unknown backup storage %s", backupRequest.Spec.BackupStorageName)
		}

		if len(availableRepoNames) == 0 {
			return []crunchyv1beta1.PGBackRestRepo{}, map[string]string{}, []byte{}, errors.New("exceeded max number of repos")
		}

		repoName := availableRepoNames[0]
		availableRepoNames = removeRepoName(availableRepoNames, repoName)
		repo, err := genPGBackrestRepo(repoName, backupStorage, nil)
		if err != nil {
			return []crunchyv1beta1.PGBackRestRepo{}, map[string]string{}, []byte{}, err
		}
		reposReconciled.PushBack(repo)

		// Keep track of backup storages which are already in use by a repo
		backupStoragesInRepos[backupRequest.Spec.BackupStorageName] = struct{}{}

		pgBackRestGlobal[fmt.Sprintf(pgBackRestPathTmpl, repo.Name)] = "/" + common.BackupStoragePrefix(db)
		if verify := pointer.Get(backupStorage.VerifyTLS); !verify {
			// See: https://pgbackrest.org/configuration.html#section-repository/option-repo-storage-verify-tls
			pgBackRestGlobal[fmt.Sprintf(pgBackRestStorageVerifyTmpl, repo.Name)] = "n"
		}
		if forcePathStyle := pointer.Get(backupStorages[repo.Name].ForcePathStyle); !forcePathStyle {
			// See: https://pgbackrest.org/configuration.html#section-repository/option-repo-s3-uri-style
			pgBackRestGlobal[fmt.Sprintf(pgBackRestStorageForcePathTmpl, repo.Name)] = pgBackRestStoragePathStyle
		}
		sType, err := backupStorageTypeFromBackrestRepo(repo)
		if err != nil {
			return []crunchyv1beta1.PGBackRestRepo{},
				map[string]string{},
				[]byte{},
				err
		}

		err = addBackupStorageCredentialsToPGBackrestSecretIni(sType, pgBackRestSecretIni, repo.Name, backupStoragesSecrets[backupRequest.Spec.BackupStorageName])
		if err != nil {
			return []crunchyv1beta1.PGBackRestRepo{},
				map[string]string{},
				[]byte{},
				errors.Join(err, errors.New("failed to add backup storage credentials to PGBackrest secret data"))
		}
	}

	// The PG operator requires a repo to be set up in order to create
	// replicas. Without any credentials we can't set a cloud-based repo so we
	// define a PVC-backed repo in case the user doesn't define any cloud-based
	// repos. Moreover, we need to keep this repo in the list even if the user
	// defines a cloud-based repo because the PG operator will restart the
	// cluster if the only repo in the list is changed.
	newRepos := []crunchyv1beta1.PGBackRestRepo{
		{
			Name: "repo1",
			Volume: &crunchyv1beta1.RepoPVC{
				VolumeClaimSpec: pgInitPVCSpec,
			},
		},
	}
	pgBackRestGlobal["repo1-retention-full"] = "1"

	// Add the reconciled repos to the list of repos
	for e := reposReconciled.Front(); e != nil; e = e.Next() {
		repo, ok := e.Value.(crunchyv1beta1.PGBackRestRepo)
		if !ok {
			return []crunchyv1beta1.PGBackRestRepo{}, map[string]string{}, []byte{}, fmt.Errorf("failed to cast repo %v", e.Value)
		}
		newRepos = append(newRepos, repo)
	}

	pgBackrestSecretBuf := new(bytes.Buffer)
	if _, err = pgBackRestSecretIni.WriteTo(pgBackrestSecretBuf); err != nil {
		return []crunchyv1beta1.PGBackRestRepo{}, map[string]string{}, []byte{}, errors.Join(err, errors.New("failed to write PGBackrest secret data"))
	}

	return newRepos, pgBackRestGlobal, pgBackrestSecretBuf.Bytes(), nil
}

func backupStorageNameFromRepo(backupStorages map[string]everestv1alpha1.BackupStorageSpec, repo crunchyv1beta1.PGBackRestRepo) string {
	for name, spec := range backupStorages {
		if repo.S3 != nil &&
			repo.S3.Bucket == spec.Bucket &&
			repo.S3.Region == spec.Region &&
			repo.S3.Endpoint == spec.EndpointURL {
			return name
		}

		if repo.Azure != nil && repo.Azure.Container == spec.Bucket {
			return name
		}
	}

	return ""
}

func removeRepoName(s []string, n string) []string {
	for i, name := range s {
		if name == n {
			return removeString(s, i)
		}
	}
	return s
}

func removeString(s []string, i int) []string {
	return append(s[:i], s[i+1:]...)
}

func backupStorageTypeFromBackrestRepo(repo crunchyv1beta1.PGBackRestRepo) (everestv1alpha1.BackupStorageType, error) {
	if repo.S3 != nil {
		return everestv1alpha1.BackupStorageTypeS3, nil
	}

	if repo.Azure != nil {
		return everestv1alpha1.BackupStorageTypeAzure, nil
	}

	return "", errors.New("could not determine backup storage type from repo")
}

func getPGRetentionCopies(retentionCopies int32) int {
	copies := int(retentionCopies)
	if copies == 0 {
		// Zero copies means unlimited. PGBackRest supports values 1-9999999
		copies = 9999999
	}

	return copies
}

// genPGBackrestRepo generates a PGBackrest repo for a given backup storage.
func genPGBackrestRepo(
	repoName string,
	backupStorage everestv1alpha1.BackupStorageSpec,
	backupSchedule *string,
) (crunchyv1beta1.PGBackRestRepo, error) {
	pgRepo := crunchyv1beta1.PGBackRestRepo{
		Name: repoName,
	}

	if backupSchedule != nil {
		pgRepo.BackupSchedules = &crunchyv1beta1.PGBackRestBackupSchedules{
			Full: backupSchedule,
		}
	}

	switch backupStorage.Type {
	case everestv1alpha1.BackupStorageTypeS3:
		pgRepo.S3 = &crunchyv1beta1.RepoS3{
			Bucket:   backupStorage.Bucket,
			Region:   backupStorage.Region,
			Endpoint: backupStorage.EndpointURL,
		}
	case everestv1alpha1.BackupStorageTypeAzure:
		pgRepo.Azure = &crunchyv1beta1.RepoAzure{
			Container: backupStorage.Bucket,
		}
	default:
		return crunchyv1beta1.PGBackRestRepo{}, fmt.Errorf("unsupported backup storage type %s", backupStorage.Type)
	}

	return pgRepo, nil
}
