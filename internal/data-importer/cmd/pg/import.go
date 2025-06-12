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
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	everestv1alpha1 "github.com/percona/everest-operator/api/v1alpha1"
	"github.com/percona/everest-operator/api/v1alpha1/dataimporterspec"
	"github.com/percona/everest-operator/internal/consts"
	pgv2 "github.com/percona/percona-postgresql-operator/pkg/apis/pgv2.percona.com/v2"
	crunchyv1beta1 "github.com/percona/percona-postgresql-operator/pkg/apis/postgres-operator.crunchydata.com/v1beta1"
	"github.com/rs/zerolog/log"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
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

	log.Info().Msgf("Importing PostgreSQL data from %s to %s/%s", backupPath, namespace, dbName)

	baseBackupName, dbDir := parseBackupPath(backupPath)

	// prepare API scheme.
	scheme := runtime.NewScheme()
	everestv1alpha1.AddToScheme(scheme)
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

	var (
		pgBackRestSecretName = "data-import-" + dbName
		restoreName          = "data-import-" + dbName
	)

	// always clean up on exit even if we fail at some point.
	defer func() {
		if cleanupErr := cleanup(ctx, k8sClient, namespace, restoreName, pgBackRestSecretName); cleanupErr != nil {
			log.Error().Err(cleanupErr).Msg("Failed to clean up after PG import")
			err = errors.Join(err, cleanupErr)
		}
	}()

	if err := preparePGBackrestSecret(ctx, k8sClient, repoName, pgBackRestSecretName, accessKeyId,
		secretAccessKey, dbName, namespace); err != nil {
		return fmt.Errorf("failed to prepare PGBackrest secret: %w", err)
	}

	if err := preparePGBackrestRepo(ctx,
		k8sClient, repoName, pgBackRestSecretName,
		dbDir, uriStyle, cfg.Source.S3.Bucket, endpoint, region,
		dbName, namespace); err != nil {
		return fmt.Errorf("failed to prepare PGBackrest repo: %w", err)
	}

	if err := runPGRestoreAndWait(ctx, k8sClient, baseBackupName, restoreName, repoName, dbName, namespace); err != nil {
		return fmt.Errorf("failed to run PG restore: %w", err)
	}

	log.Info().Msgf("Successfully imported PostgreSQL data from %s to %s/%s", backupPath, namespace, dbName)
	return nil
}

// returns: [baseBackupName, DBDirectory]
func parseBackupPath(fullPath string) (string, string) {
	// for consistency
	fullPath = strings.TrimSuffix(fullPath, "/")
	if !strings.HasPrefix(fullPath, "/") {
		fullPath = "/" + fullPath
	}
	base := filepath.Base(fullPath)
	fullPath = strings.TrimSuffix(fullPath, "backup/db/"+base)
	return base, fullPath
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
	repoIdx := len(pg.Spec.Backups.PGBackRest.Repos) + 1
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
	annots[consts.PauseReconcileAnnotation] = consts.PauseReconcileAnnotationValueTrue
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
	delete(annots, consts.PauseReconcileAnnotation)
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
	secretName string,
	accessKeyId, secretAccessKey string,
	dbName, namespace string,
) error {
	content := fmt.Sprintf(pgBackrestSecretContentTpl, repoName, accessKeyId, repoName, secretAccessKey)
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
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
	secretName string,
	dbDirPath string,
	uriStyle string,
	bucket, endpoint, region string,
	dbName, namespace string,
) error {
	pg := &pgv2.PerconaPGCluster{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: dbName}, pg); err != nil {
		return fmt.Errorf("failed to get PerconaPGCluster %s/%s: %w", namespace, dbName, err)
	}

	// configure PGBackRest secret
	pg.Spec.Backups.PGBackRest.Configuration = append(pg.Spec.Backups.PGBackRest.Configuration, corev1.VolumeProjection{
		Secret: &corev1.SecretProjection{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: secretName,
			},
		},
	})

	if pg.Spec.Backups.PGBackRest.Global == nil {
		pg.Spec.Backups.PGBackRest.Global = make(map[string]string)
	}

	// configure global settings
	pg.Spec.Backups.PGBackRest.Global[fmt.Sprintf("%s-path", repoName)] = dbDirPath
	pg.Spec.Backups.PGBackRest.Global[fmt.Sprintf("%s-s3-uri-style", repoName)] = uriStyle

	// configure PG repo
	pg.Spec.Backups.PGBackRest.Repos = append(pg.Spec.Backups.PGBackRest.Repos, crunchyv1beta1.PGBackRestRepo{
		Name: repoName,
		S3: &crunchyv1beta1.RepoS3{
			Bucket:   bucket,
			Endpoint: endpoint,
			Region:   region,
		},
	})

	// retry update operation to reduce chances of conflicts
	bo := backoff.WithContext(backoff.NewExponentialBackOff(), ctx)
	err := backoff.Retry(func() error {
		return c.Update(ctx, pg)
	}, bo)
	return err
}

const defaultRetryInterval = time.Second * 30

func runPGRestoreAndWait(
	ctx context.Context,
	c client.Client,
	backupName string,
	restoreName string,
	repoName string,
	dbName, namespace string,
) error {
	pgRestore := &pgv2.PerconaPGRestore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      restoreName,
			Namespace: namespace,
		},
	}
	if _, err := controllerutil.CreateOrUpdate(ctx, c, pgRestore, func() error {
		pgRestore.Spec = pgv2.PerconaPGRestoreSpec{
			PGCluster: dbName,
			RepoName:  repoName,
			Options: []string{
				"--type=immediate", // TODO: support PITR
				"--set=" + backupName,
			},
		}
		return nil
	}); err != nil {
		return err
	}
	// wait for the restore to complete
	return wait.PollUntilContextCancel(
		ctx,
		defaultRetryInterval,
		false,
		func(ctx context.Context) (done bool, err error) {
			pg := &pgv2.PerconaPGRestore{}
			if err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: pgRestore.Name}, pg); err != nil {
				return false, fmt.Errorf("failed to get PerconaPGRestore %s/%s: %w", namespace, pgRestore.Name, err)
			}
			return pg.Status.State == pgv2.RestoreSucceeded, nil
		},
	)
}

func cleanup(
	ctx context.Context,
	c client.Client,
	namespace,
	pgRestoreName,
	secretName string,
) error {
	// delete PG restore
	pgRestore := &pgv2.PerconaPGRestore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pgRestoreName,
			Namespace: namespace,
		},
	}
	if err := c.Delete(ctx, pgRestore); client.IgnoreNotFound(err) != nil {
		return fmt.Errorf("failed to delete PerconaPGRestore %s/%s: %w", namespace, pgRestoreName, err)
	}
	// delete PGBackrest secret
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
		},
	}
	if err := c.Delete(ctx, secret); client.IgnoreNotFound(err) != nil {
		return fmt.Errorf("failed to delete secret %s/%s: %w", namespace, secretName, err)
	}
	return nil
}
