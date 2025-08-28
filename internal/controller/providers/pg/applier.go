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
	"context"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"regexp"
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
	"github.com/percona/everest-operator/internal/consts"
	"github.com/percona/everest-operator/internal/controller/common"
)

const (
	pgBackupTypeDate      = "time"
	pgBackupTypeImmediate = "immediate"

	pgBackRestPathTmpl             = "%s-path"
	pgBackRestRetentionTmpl        = "%s-retention-full"
	pgBackRestStorageVerifyTmpl    = "%s-storage-verify-tls"
	pgBackRestStorageForcePathTmpl = "%s-s3-uri-style"
	pgBackRestStoragePathStyle     = "path"
	repo1Name                      = "repo1"
)

type applier struct {
	*Provider
	ctx context.Context //nolint:containedctx
}

func (p *applier) Paused(paused bool) {
	p.PerconaPGCluster.Spec.Pause = &paused
}

func (p *applier) AllowUnsafeConfig() {
}

func (p *applier) Metadata() error {
	if p.PerconaPGCluster.GetDeletionTimestamp().IsZero() {
		for _, f := range []string{
			finalizerDeletePGPVC,
			finalizerDeletePGSSL,
		} {
			controllerutil.AddFinalizer(p.PerconaPGCluster, f)
		}
	}
	return nil
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
			Name:      consts.PGDeploymentName,
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
	pg.Spec.ImagePullPolicy = corev1.PullIfNotPresent
	pgMajorVersionMatch := regexp.
		MustCompile(`^(\d+)`).
		FindStringSubmatch(database.Spec.Engine.Version)
	if len(pgMajorVersionMatch) < 2 { //nolint:mnd
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

	var currentInstSet *pgv2.PGInstanceSetSpec
	if len(p.currentPGSpec.InstanceSets) > 0 {
		currentInstSet = &p.currentPGSpec.InstanceSets[0]
	}
	if err := configureStorage(p.ctx, p.C, &pg.Spec.InstanceSets[0], currentInstSet, p.DB); err != nil {
		return fmt.Errorf("failed to configure storage: %w", err)
	}

	image, err := common.GetOperatorImage(p.ctx, p.C, types.NamespacedName{
		Name:      consts.PGDeploymentName,
		Namespace: p.DB.GetNamespace(),
	})
	if err != nil {
		return err
	}
	pg.Spec.Extensions = pgv2.ExtensionsSpec{
		Image:           image,
		ImagePullPolicy: corev1.PullIfNotPresent,
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
		if annotations, ok := consts.ExposeAnnotationsMap[p.clusterType]; ok {
			pg.Spec.Proxy.PGBouncer.ServiceExpose.Metadata.Annotations = annotations
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
	monitoring, err := common.GetDBMonitoringConfig(p.ctx, p.C, p.DB)
	if err != nil {
		return err
	}
	p.PerconaPGCluster.Spec.PMM = defaultSpec().PMM
	if monitoring.Spec.Type == everestv1alpha1.PMMMonitoringType {
		if err := p.applyPMMCfg(monitoring); err != nil {
			return err
		}
	}
	return nil
}

func (p *applier) PodSchedulingPolicy() error {
	// NOTE: this method shall be called after Engine() and Proxy() methods
	// because it extends the engine and proxy specs with the affinity rules.
	//
	// The following cases are possible:
	// 1. The user did not specify a .spec.podSchedulingPolicyName -> do nothing.
	// 2. The user specified a .spec.podSchedulingPolicyName, but it does not exist -> return error.
	// 3. The user specified a .spec.podSchedulingPolicyName, and it exists, but it is not applicable to the upstream cluster -> return error.
	// 4. The user specified a .spec.podSchedulingPolicyName, and it exists, but affinity configuration is absent there -> do nothing.
	// 5. The user specified a .spec.podSchedulingPolicyName, and it exists, and it is applicable to the upstream cluster ->
	// copy the affinity rules to the upstream cluster spec from policy.

	pg := p.PerconaPGCluster
	// --------------------------------- //
	// Special workaround, need to reset all affinity params in p.PerconaPGCluster before moving further.
	// TODO: Remove it once https://perconadev.atlassian.net/browse/EVEREST-2023 is addressed
	for i := range len(pg.Spec.InstanceSets) {
		// all instances in sets have the same affinity configuration.
		pg.Spec.InstanceSets[i].Affinity = nil
	}
	pg.Spec.Proxy.PGBouncer.Affinity = nil
	// --------------------------------- //
	pspName := p.DB.Spec.PodSchedulingPolicyName
	if pspName == "" {
		// Covers case 1.
		return nil
	}

	psp, err := common.GetPodSchedulingPolicy(p.ctx, p.C, pspName)
	if err != nil {
		// Not found or other error.
		// Covers case 2.
		return err
	}

	if psp.Spec.EngineType != everestv1alpha1.DatabaseEnginePostgresql {
		// Covers case 3.
		return fmt.Errorf("requested pod scheduling policy='%s' is not applicable to engineType='%s'",
			pspName, everestv1alpha1.DatabaseEnginePostgresql)
	}

	if !psp.HasRules() {
		// Nothing to do.
		// Covers case 4.
		// The affinity rules will be applied later once admin sets them in policy.
		return nil
	}

	// Covers case 5.
	pspAffinityConfig := psp.Spec.AffinityConfig

	// Engine
	if pspAffinityConfig.PostgreSQL.Engine != nil {
		engineAffinityConfig := pspAffinityConfig.PostgreSQL.Engine.DeepCopy()
		for i := range len(pg.Spec.InstanceSets) {
			pg.Spec.InstanceSets[i].Affinity = engineAffinityConfig
		}
	}

	// Proxy
	if pspAffinityConfig.PostgreSQL.Proxy != nil {
		pg.Spec.Proxy.PGBouncer.Affinity = pspAffinityConfig.PostgreSQL.Proxy.DeepCopy()
	}

	return nil
}

func (p *applier) applyPMMCfg(monitoring *everestv1alpha1.MonitoringConfig) error {
	pg := p.PerconaPGCluster
	database := p.DB
	c := p.C
	ctx := p.ctx

	pg.Spec.PMM = &pgv2.PMMSpec{
		Enabled:         true,
		Resources:       common.GetPMMResources(pointer.Get(database.Spec.Monitoring), database.Spec.Engine.Size()),
		Secret:          fmt.Sprintf("%s%s-pmm", consts.EverestSecretsPrefix, database.GetName()),
		Image:           common.DefaultPMMClientImage,
		ImagePullPolicy: corev1.PullIfNotPresent,
	}

	if monitoring.Spec.PMM.Image != "" {
		pg.Spec.PMM.Image = monitoring.Spec.PMM.Image
	}
	pmmURL, err := url.Parse(monitoring.Spec.PMM.URL)
	if err != nil {
		return errors.Join(err, errors.New("invalid monitoring URL"))
	}
	pg.Spec.PMM.ServerHost = pmmURL.Hostname()

	apiKey, err := common.GetSecretFromMonitoringConfig(ctx, c, monitoring)
	if err != nil {
		return err
	}

	if err := common.CreateOrUpdateSecretData(ctx, c, database, pg.Spec.PMM.Secret,
		map[string][]byte{
			"PMM_SERVER_KEY": []byte(apiKey),
		},
		true,
	); err != nil {
		return err
	}
	return nil
}

func defaultSpec() pgv2.PerconaPGClusterSpec {
	return pgv2.PerconaPGClusterSpec{
		InstanceSets: pgv2.PGInstanceSets{
			{
				Name: "instance1",
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{},
				},
			},
		},
		PMM: nil,
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

	// NOTE: The PG DataImporter depends on Everest creating this Secret.
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
					// NOTE: The PG DataImporter depends on Everest setting this field.
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

	backupStorage, err := common.GetBackupStorage(ctx, c, backupStorageName, database.GetNamespace())
	if err != nil {
		return nil, err
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
// NOTE: The PG DataImporter depends on Everest creating this Secret.
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
	backupStorages map[string]everestv1alpha1.BackupStorage,
	backupStoragesSecrets map[string]*corev1.Secret,
) error {
	ctx := p.ctx
	c := p.C
	for _, restore := range restoreList.Items {
		// If the restore has already completed, skip it.
		if restore.IsComplete() {
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

		backupStorage, err := common.GetBackupStorage(ctx, c, backupStorageName, restore.GetNamespace())
		if err != nil {
			return err
		}

		// Get the Secret used by the BackupStorage.
		backupStorageSecret := &corev1.Secret{}
		key := types.NamespacedName{Name: backupStorage.Spec.CredentialsSecretName, Namespace: backupStorage.GetNamespace()}
		err = c.Get(ctx, key, backupStorageSecret)
		if err != nil {
			return errors.Join(err,
				fmt.Errorf("failed to get backup storage secret %s", backupStorage.Spec.CredentialsSecretName))
		}

		backupStorages[backupStorage.Name] = *backupStorage
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
	backupStorages map[string]everestv1alpha1.BackupStorage,
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

		backupStorage, err := common.GetBackupStorage(ctx, c, schedule.BackupStorageName, database.GetNamespace())
		if err != nil {
			return err
		}

		backupStorageSecret := &corev1.Secret{}
		err = c.Get(ctx, types.NamespacedName{Name: backupStorage.Spec.CredentialsSecretName, Namespace: backupStorage.Namespace}, backupStorageSecret)
		if err != nil {
			return errors.Join(err, fmt.Errorf("failed to get backup storage secret %s", backupStorage.Spec.CredentialsSecretName))
		}

		backupStorages[backupStorage.Name] = *backupStorage
		backupStoragesSecrets[backupStorage.Name] = backupStorageSecret
	}
	return nil
}

// Add backup storages used by on-demand backups to the list.
func (p *applier) addBackupStoragesByOnDemandBackups(
	backupList *everestv1alpha1.DatabaseClusterBackupList,
	backupStorages map[string]everestv1alpha1.BackupStorage,
	backupStoragesSecrets map[string]*corev1.Secret,
) error {
	ctx := p.ctx
	c := p.C
	for _, backup := range backupList.Items {
		// Check if we already fetched that backup storage
		if _, ok := backupStorages[backup.Spec.BackupStorageName]; ok {
			continue
		}

		backupStorage, err := common.GetBackupStorage(ctx, c, backup.Spec.BackupStorageName, backup.GetNamespace())
		if err != nil {
			return err
		}

		backupStorageSecret := &corev1.Secret{}
		err = c.Get(ctx, types.NamespacedName{Name: backupStorage.Spec.CredentialsSecretName, Namespace: backupStorage.Namespace}, backupStorageSecret)
		if err != nil {
			return fmt.Errorf("failed to get backup storage secret '%s': %w", backupStorage.Spec.CredentialsSecretName, err)
		}

		backupStorages[backupStorage.Name] = *backupStorage
		backupStoragesSecrets[backupStorage.Name] = backupStorageSecret
	}
	return nil
}

func (p *applier) reconcilePGBackupsSpec() (pgv2.Backups, error) {
	ctx := p.ctx
	c := p.C
	database := p.DB
	engine := p.DBEngine
	oldBackups := p.PerconaPGCluster.Spec.Backups

	pgbackrestVersion, ok := engine.Status.AvailableVersions.Backup[database.Spec.Engine.Version]
	if !ok {
		return pgv2.Backups{}, fmt.Errorf("pgbackrest version %s not available", database.Spec.Engine.Version)
	}

	newBackups := pgv2.Backups{
		PGBackRest: pgv2.PGBackRestArchive{
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
	backupList, err := common.DatabaseClusterBackupsThatReferenceObject(ctx, c, consts.DBClusterBackupDBClusterNameField, database.GetNamespace(), database.Name)
	if err != nil {
		return pgv2.Backups{}, err
	}

	backupStorages := map[string]everestv1alpha1.BackupStorage{}
	backupStoragesSecrets := map[string]*corev1.Secret{}

	// Add backup storages used by on-demand backups to the list
	if err := p.addBackupStoragesByOnDemandBackups(backupList, backupStorages, backupStoragesSecrets); err != nil {
		return pgv2.Backups{}, err
	}

	// List DatabaseClusterRestore objects for this database
	restoreList, err := common.DatabaseClusterRestoresThatReferenceObject(ctx, c, consts.DBClusterRestoreDBClusterNameField, database.GetNamespace(), database.GetName())
	if err != nil {
		return pgv2.Backups{}, err
	}

	// Add backup storages used by restores to the list
	if err := p.addBackupStoragesByRestores(backupList, restoreList, backupStorages, backupStoragesSecrets); err != nil {
		return pgv2.Backups{}, err
	}

	backupSchedules := database.Spec.Backup.Schedules

	// Add backup storages used by backup schedules to the list
	if err := p.addBackupStoragesBySchedules(backupSchedules, backupStorages, backupStoragesSecrets); err != nil {
		return pgv2.Backups{}, err
	}

	pgBackrestRepos, pgBackrestGlobal, pgBackrestSecretData, err := reconcilePGBackRestRepos(
		oldBackups.PGBackRest.Repos,
		backupSchedules,
		backupList.Items,
		backupStorages,
		backupStoragesSecrets,
		database.Spec.Engine.Storage,
		database,
	)
	if err != nil {
		return pgv2.Backups{}, err
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
		return pgv2.Backups{}, err
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

func reconcilePGBackRestRepos(
	oldRepos []crunchyv1beta1.PGBackRestRepo,
	backupSchedules []everestv1alpha1.BackupSchedule,
	backups []everestv1alpha1.DatabaseClusterBackup,
	backupStorages map[string]everestv1alpha1.BackupStorage,
	backupStoragesSecrets map[string]*corev1.Secret,
	engineStorage everestv1alpha1.Storage,
	db *everestv1alpha1.DatabaseCluster,
) (
	[]crunchyv1beta1.PGBackRestRepo,
	map[string]string,
	[]byte,
	error,
) {
	reposReconciler, err := newPGReposReconciler(oldRepos, backupSchedules)
	if err != nil {
		return []crunchyv1beta1.PGBackRestRepo{}, map[string]string{}, []byte{},
			errors.Join(err, errors.New("failed to initialize PGBackrest repos reconciler"))
	}

	err = reposReconciler.reconcileExistingSchedules(backupStorages, backupStoragesSecrets, db)
	if err != nil {
		return []crunchyv1beta1.PGBackRestRepo{}, map[string]string{}, []byte{},
			errors.Join(err, errors.New("failed to reconcile schedules"))
	}

	err = reposReconciler.reconcileRepos(backupStorages, backupStoragesSecrets, db)
	if err != nil {
		return []crunchyv1beta1.PGBackRestRepo{}, map[string]string{}, []byte{},
			errors.Join(err, errors.New("failed to reconcile repos"))
	}

	err = reposReconciler.reconcileBackups(backups, backupStorages, backupStoragesSecrets, db)
	if err != nil {
		return []crunchyv1beta1.PGBackRestRepo{}, map[string]string{}, []byte{}, err
	}

	err = reposReconciler.addNewSchedules(backupStorages, backupStoragesSecrets, db)
	if err != nil {
		return []crunchyv1beta1.PGBackRestRepo{}, map[string]string{}, []byte{},
			errors.Join(err, errors.New("failed to add new backup schedules"))
	}

	newRepos, err := reposReconciler.addDefaultRepo(engineStorage)
	if err != nil {
		return []crunchyv1beta1.PGBackRestRepo{}, map[string]string{}, []byte{}, err
	}

	pgBackrestIniBytes, err := reposReconciler.pgBackRestIniBytes()
	if err != nil {
		return []crunchyv1beta1.PGBackRestRepo{}, map[string]string{}, []byte{}, err
	}

	return newRepos, reposReconciler.pgBackRestGlobal, pgBackrestIniBytes, nil
}

func backupStorageNameFromRepo(backupStorages map[string]everestv1alpha1.BackupStorage, repo crunchyv1beta1.PGBackRestRepo, dbNamespace string) string {
	for name, bs := range backupStorages {
		if repo.S3 != nil &&
			dbNamespace == bs.Namespace &&
			repo.S3.Bucket == bs.Spec.Bucket &&
			repo.S3.Region == bs.Spec.Region &&
			repo.S3.Endpoint == bs.Spec.EndpointURL {
			return name
		}

		if repo.Azure != nil && repo.Azure.Container == bs.Spec.Bucket {
			return name
		}
	}

	return ""
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

func configureStorage(
	ctx context.Context,
	c client.Client,
	desired *pgv2.PGInstanceSetSpec,
	current *pgv2.PGInstanceSetSpec,
	db *everestv1alpha1.DatabaseCluster,
) error {
	var currentSize resource.Quantity
	if current != nil {
		currentSize = current.DataVolumeClaimSpec.Resources.Requests[corev1.ResourceStorage]
	}

	setStorageSize := func(size resource.Quantity) {
		desired.DataVolumeClaimSpec = corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteOnce,
			},
			StorageClassName: db.Spec.Engine.Storage.Class,
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: size,
				},
			},
		}
	}

	return common.ConfigureStorage(ctx, c, db, currentSize, setStorageSize)
}
