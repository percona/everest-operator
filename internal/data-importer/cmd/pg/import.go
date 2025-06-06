package pg

import (
	"context"
	"errors"
	"fmt"

	everestv1alpha1 "github.com/percona/everest-operator/api/v1alpha1"
	"github.com/percona/everest-operator/api/v1alpha1/dataimporterspec"
	pgv2 "github.com/percona/percona-postgresql-operator/pkg/apis/pgv2.percona.com/v2"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

var Cmd = &cobra.Command{
	Use:  "pg",
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		configPath := args[0]
		if err := runPGImport(cmd.Context(), configPath); err != nil {
			log.Error().Err(err).Msg("Failed to run pg import")
			panic(err)
		}
	},
}

func runPGImport(
	ctx context.Context,
	configPath string,
) (err error) {
	cfg := &dataimporterspec.Spec{}
	if err := cfg.ReadFromFilepath(configPath); err != nil {
		return err
	}

	var (
		dbName          = cfg.Target.DatabaseClusterRef.Name
		namespace       = cfg.Target.DatabaseClusterRef.Namespace
		accessKeyId     = cfg.Source.S3.AccessKeyID
		secretAccessKey = cfg.Source.S3.SecretKey
		endpoint        = cfg.Source.S3.EndpointURL
		region          = cfg.Source.S3.Region
		backupPath      = cfg.Source.Path
		uriStyle        = "host"
	)
	if cfg.Source.S3.ForcePathStyle {
		uriStyle = "path"
	}

	// prepare API scheme.
	scheme := runtime.NewScheme()
	pgv2.AddToScheme(scheme)
	corev1.AddToScheme(scheme)

	// prepare k8s client.
	k8sClient, err := client.New(config.GetConfigOrDie(), client.Options{Scheme: scheme})
	if err != nil {
		return err
	}

	// first we will pause reconciliation of the database cluster because
	// we will be making changes to the PGCluster and we don't want Everest interfering.
	if err := pauseDBReconciliation(ctx, k8sClient, dbName, namespace); err != nil {
		return fmt.Errorf("failed to pause DB reconciliation: %w", err)
	}
	defer func() {
		if unpauseErr := unpauseDBReconciliation(ctx, k8sClient, dbName, namespace); unpauseErr != nil {
			log.Error().Err(err).Msg("Failed to unpause DB reconciliation")
			err = errors.Join(err, unpauseErr)
		}
	}()

	repoName, err := getRepoName(ctx, k8sClient, dbName, namespace)
	if err != nil {
		return fmt.Errorf("failed to get repo name: %w", err)
	}

	if err := preparePGBackrestSecret(ctx, k8sClient, repoName, accessKeyId,
		secretAccessKey, dbName, namespace); err != nil {
		return fmt.Errorf("failed to prepare PGBackrest secret: %w", err)
	}

	if err := preparePGBackrestRepo(ctx, k8sClient, repoName,
		backupPath, uriStyle, cfg.Source.S3.Bucket, endpoint, region,
		dbName, namespace); err != nil {
		return fmt.Errorf("failed to prepare PGBackrest repo: %w", err)
	}

	if err := runPGRestoreAndWait(ctx, k8sClient, backupPath, repoName, dbName, namespace); err != nil {
		return fmt.Errorf("failed to run PG restore: %w", err)
	}

	return nil
}

func getRepoName(
	ctx context.Context,
	c client.Client,
	dbName, namespace string,
) (string, error) {
	pg := &pgv2.PerconaPGCluster{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: dbName}, pg); err != nil {
		return "", fmt.Errorf("failed to get PerconaPGCluster %s/%s: %w", namespace, dbName, err)
	}
	repoIdx := len(pg.Spec.Backups.PGBackRest.Repos) - 1
	if repoIdx < 0 {
		repoIdx = 0
	}
	return fmt.Sprintf("repo%d", repoIdx), nil
}

func pauseDBReconciliation(
	ctx context.Context,
	c client.Client,
	name, namespace string) error {
	db := &everestv1alpha1.DatabaseCluster{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, db); err != nil {
		return fmt.Errorf("failed to get database cluster %s/%s: %w", namespace, name, err)
	}
	annots := db.GetAnnotations()
	if annots == nil {
		annots = make(map[string]string)
	}
	annots["everest.percona.com/reconcile-paused"] = "true"
	db.SetAnnotations(annots)
	if err := c.Update(ctx, db); err != nil {
		return fmt.Errorf("failed to pause reconciliation for database cluster %s/%s: %w", namespace, name, err)
	}
	return nil
}

func unpauseDBReconciliation(
	ctx context.Context,
	c client.Client,
	name, namespace string) error {
	db := &everestv1alpha1.DatabaseCluster{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, db); err != nil {
		return fmt.Errorf("failed to get database cluster %s/%s: %w", namespace, name, err)
	}
	annots := db.GetAnnotations()
	if annots == nil {
		annots = make(map[string]string)
	}
	delete(annots, "everest.percona.com/reconcile-paused")
	db.SetAnnotations(annots)
	if err := c.Update(ctx, db); err != nil {
		return fmt.Errorf("failed to unpause reconciliation for database cluster %s/%s: %w", namespace, name, err)
	}
	return nil
}

const pgBackrestSecretContentTpl = `
[global]
%s-s3-key=%s
%s-s3-key-secret=%s
`

func preparePGBackrestSecret(
	ctx context.Context,
	c client.Client,
	repoName string,
	accessKeyId, secretAccessKey string,
	dbName, namespace string,
) error {
	content := fmt.Sprintf(pgBackrestSecretContentTpl, repoName, accessKeyId, repoName, secretAccessKey)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "data-import-" + dbName,
			Namespace: namespace,
		},
	}
	if _, err := controllerutil.CreateOrUpdate(ctx, c, secret, func() error {
		secret.StringData = map[string]string{
			"s3.conf": content,
		}
		return nil
	}); err != nil {
		return fmt.Errorf("failed to create or update PGBackrest secret: %w", err)
	}
	return nil
}

func preparePGBackrestRepo(
	ctx context.Context,
	c client.Client,
	repoName string,
	path string,
	uriStyle string,
	bucket, endpoint, region string,
	dbName, namespace string,
) error {
	return nil
}

func runPGRestoreAndWait(
	ctx context.Context,
	c client.Client,
	backupPath string,
	repoName string,
	dbName, namespace string,
) error {
	return nil
}
