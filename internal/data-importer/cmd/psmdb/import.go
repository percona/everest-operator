package psmdb

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/percona/everest-operator/api/v1alpha1/dataimporterspec"
	pbmdefs "github.com/percona/percona-backup-mongodb/pbm/defs"
	psmdbv1 "github.com/percona/percona-server-mongodb-operator/pkg/apis/psmdb/v1"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

var Cmd = &cobra.Command{
	Use:  "psmdb",
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		configPath := args[0]
		if err := runPSMDBImport(cmd.Context(), configPath); err != nil {
			log.Error().Err(err).Msg("Failed to run psmdb import")
			panic(err)
		}
	},
}

func runPSMDBImport(ctx context.Context, configPath string) error {
	cfg := &dataimporterspec.Spec{}
	if err := cfg.ReadFromFilepath(configPath); err != nil {
		return err
	}

	// prepare API scheme.
	scheme := runtime.NewScheme()
	corev1.AddToScheme(scheme)
	psmdbv1.SchemeBuilder.AddToScheme(scheme)

	var (
		dbName          = cfg.Target.DatabaseClusterRef.Name
		namespace       = cfg.Target.DatabaseClusterRef.Namespace
		accessKeyId     = cfg.Source.S3.AccessKeyID
		secretAccessKey = cfg.Source.S3.SecretKey
		endpoint        = cfg.Source.S3.EndpointURL
		region          = cfg.Source.S3.Region
		bucket          = cfg.Source.S3.Bucket
		backupPath      = cfg.Source.Path
	)

	// prepare k8s client.
	k8sClient, err := client.New(config.GetConfigOrDie(), client.Options{Scheme: scheme})
	if err != nil {
		return err
	}
	var (
		psmdbRestoreName = "data-import-" + dbName
	)
	defer cleanup(ctx, k8sClient, namespace, psmdbRestoreName)

	log.Info().Msgf("Starting PSMDB import for database %s in namespace %s", dbName, namespace)

	// Prepare S3 credentials secret.
	if err := prepareS3CredentialSecret(k8sClient, ctx, psmdbRestoreName, namespace, accessKeyId, secretAccessKey); err != nil {
		return err
	}
	log.Info().Msgf("S3 credentials secret %s created in namespace %s", psmdbRestoreName, namespace)

	// Run PSMDB restore and wait for it to complete.
	log.Info().Msgf("Starting PSMDB restore for database %s from backup path %s", dbName, backupPath)
	if err := runPSMDBRestoreAndWait(ctx, k8sClient, namespace, dbName, psmdbRestoreName, backupPath, bucket, endpoint, region); err != nil {
		return fmt.Errorf("failed to run PSMDB restore: %w", err)
	}
	log.Info().Msgf("PSMDB restore %s completed successfully for database %s", psmdbRestoreName, dbName)
	return nil
}

func prepareS3CredentialSecret(
	c client.Client,
	ctx context.Context,
	psmdbRestoreName, namespace string,
	accessKeyId, secretAccessKey string,
) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      psmdbRestoreName,
			Namespace: namespace,
		},
	}
	if _, err := controllerutil.CreateOrUpdate(ctx, c, secret, func() error {
		secret.Type = corev1.SecretTypeOpaque
		secret.StringData = map[string]string{
			"AWS_ACCESS_KEY_ID":     accessKeyId,
			"AWS_SECRET_ACCESS_KEY": secretAccessKey,
		}
		return nil
	}); err != nil {
		return err
	}
	return nil
}

func runPSMDBRestoreAndWait(
	ctx context.Context,
	c client.Client,
	namespace string,
	dbName string,
	psmdbRestoreName string,
	backupPath string,
	bucket, endpoint, region string,
) error {
	// parse the backup path to extract prefix and destination.
	backupPath = strings.Trim(backupPath, "/")
	split := strings.Split(backupPath, "/")
	prefix := strings.Join(split[:len(split)-1], "/")
	destination := fmt.Sprintf("s3://%s/%s", bucket, backupPath)

	psmdbRestore := &psmdbv1.PerconaServerMongoDBRestore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      psmdbRestoreName,
			Namespace: namespace,
		},
	}

	if _, err := controllerutil.CreateOrUpdate(ctx, c, psmdbRestore, func() error {
		psmdbRestore.Spec = psmdbv1.PerconaServerMongoDBRestoreSpec{
			ClusterName: dbName,
			BackupSource: &psmdbv1.PerconaServerMongoDBBackupStatus{
				Type:        pbmdefs.LogicalBackup,
				Destination: destination,
				S3: &psmdbv1.BackupStorageS3Spec{
					Bucket:            bucket,
					Region:            region,
					EndpointURL:       endpoint,
					CredentialsSecret: psmdbRestoreName,
					Prefix:            prefix,
				},
			},
		}
		return nil
	}); err != nil {
		return fmt.Errorf("failed to create or update PSMDB restore: %w", err)
	}

	// wait for it to be completed.
	return wait.PollUntilContextCancel(ctx, 5*time.Second, true, func(ctx context.Context) (bool, error) {
		psmdbRestore := &psmdbv1.PerconaServerMongoDBRestore{}
		if err := c.Get(ctx, types.NamespacedName{
			Name:      psmdbRestoreName,
			Namespace: namespace,
		}, psmdbRestore); err != nil {
			return false, fmt.Errorf("failed to get PSMDB restore %s: %w", psmdbRestoreName, err)
		}
		return psmdbRestore.Status.State == psmdbv1.RestoreStateReady, nil
	})
}

func cleanup(
	ctx context.Context,
	c client.Client,
	namespace string,
	psmdbRestoreName string,
) error {
	// delete S3 secret.
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      psmdbRestoreName,
			Namespace: namespace,
		},
	}
	if err := c.Delete(ctx, secret); client.IgnoreNotFound(err) != nil {
		return fmt.Errorf("failed to delete S3 credentials secret %s: %w", psmdbRestoreName, err)
	}

	// delete PSMDB restore.
	restore := &psmdbv1.PerconaServerMongoDBRestore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      psmdbRestoreName,
			Namespace: namespace,
		},
	}
	if err := c.Delete(ctx, restore); client.IgnoreNotFound(err) != nil {
		return fmt.Errorf("failed to delete PSMDB restore %s: %w", psmdbRestoreName, err)
	}
	return nil
}
