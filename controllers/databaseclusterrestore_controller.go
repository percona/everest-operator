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

package controllers

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	pgv2 "github.com/percona/percona-postgresql-operator/pkg/apis/pgv2.percona.com/v2"
	psmdbv1 "github.com/percona/percona-server-mongodb-operator/pkg/apis/psmdb/v1"
	pxcv1 "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	everestv1alpha1 "github.com/percona/everest-operator/api/v1alpha1"
)

const (
	psmdbRestoreKind    = "PerconaServerMongoDBRestore"
	psmdbRestoreAPI     = "psmdb.percona.com/v1"
	psmdbRestoreCRDName = "perconaservermongodbrestores.psmdb.percona.com"
	pxcRestoreCRDName   = "perconaxtradbclusterrestores.pxc.percona.com"
	pgRestoreCRDName    = "perconapgrestores.pgv2.percona.com"
	clusterReadyTimeout = 10 * time.Minute

	dbClusterRestoreDBClusterNameField = ".spec.dbClusterName"
	pgBackupTypeImmediate              = "immediate"
	pgBackupTypeDate                   = "time"
)

var (
	errPitrTypeIsNotSupported = errors.New("unknown PITR type")
	errPitrTypeLatest         = errors.New("'latest' type is not supported by Everest yet")
	errPitrEmptyDate          = errors.New("no date provided for PITR of type 'date'")
)

// DatabaseClusterRestoreReconciler reconciles a DatabaseClusterRestore object.
type DatabaseClusterRestoreReconciler struct {
	client.Client
	Scheme          *runtime.Scheme
	systemNamespace string
}

//+kubebuilder:rbac:groups=everest.percona.com,resources=databaseclusterrestores,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=everest.percona.com,resources=databaseclusterrestores/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=everest.percona.com,resources=databaseclusterrestores/finalizers,verbs=update
// +kubebuilder:rbac:groups=pxc.percona.com,resources=perconaxtradbclusterrestores,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=psmdb.percona.com,resources=perconaservermongodbrestores,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=pgv2.percona.com,resources=perconapgrestores,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *DatabaseClusterRestoreReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)
	logger.Info("reconciling", "request", req)
	time.Sleep(time.Second)

	cr := &everestv1alpha1.DatabaseClusterRestore{}
	err := r.Get(ctx, req.NamespacedName, cr)
	if err != nil {
		// NotFound cannot be fixed by requeuing so ignore it. During background
		// deletion, we receive delete events from cluster's dependents after
		// cluster is deleted.
		if err = client.IgnoreNotFound(err); err != nil {
			logger.Error(err, "unable to fetch DatabaseClusterRestore")
		}
		return reconcile.Result{}, err
	}

	if (cr.Spec.DataSource.DBClusterBackupName == "" && cr.Spec.DataSource.BackupSource == nil) ||
		(cr.Spec.DataSource.DBClusterBackupName != "" && cr.Spec.DataSource.BackupSource != nil) {
		return reconcile.Result{}, errors.New("specify either dbClusterBackupName or backupSource")
	}

	if len(cr.ObjectMeta.Labels) == 0 {
		cr.ObjectMeta.Labels = map[string]string{
			databaseClusterNameLabel: cr.Spec.DBClusterName,
		}

		if cr.Spec.DataSource.BackupSource != nil {
			key := fmt.Sprintf(backupStorageNameLabelTmpl, cr.Spec.DataSource.BackupSource.BackupStorageName)
			cr.ObjectMeta.Labels[key] = backupStorageLabelValue
		}

		if err := r.Update(ctx, cr); err != nil {
			return reconcile.Result{}, err
		}
	}

	dbCRNamespacedName := types.NamespacedName{
		Name:      cr.Spec.DBClusterName,
		Namespace: cr.Namespace,
	}
	dbCR := &everestv1alpha1.DatabaseCluster{}
	err = r.Get(ctx, dbCRNamespacedName, dbCR)
	if err != nil {
		if err = client.IgnoreNotFound(err); err != nil {
			logger.Error(err, "unable to fetch DatabaseCluster")
		}
		return reconcile.Result{}, err
	}

	if dbCR.Spec.Engine.Type == everestv1alpha1.DatabaseEnginePXC {
		if err := r.restorePXC(ctx, cr, dbCR); err != nil {
			logger.Error(err, "unable to restore PXC Cluster")
			return reconcile.Result{}, err
		}
	}
	if dbCR.Spec.Engine.Type == everestv1alpha1.DatabaseEnginePSMDB {
		if err := r.restorePSMDB(ctx, cr); err != nil {
			// The DatabaseCluster controller is responsible for updating the
			// upstream DB cluster with the necessary storage definition. If
			// the storage is not defined in the upstream DB cluster CR, we
			// requeue the backup to give the DatabaseCluster controller a
			// chance to update the upstream DB cluster CR.
			if errors.Is(err, ErrBackupStorageUndefined) {
				return reconcile.Result{Requeue: true}, nil
			}

			logger.Error(err, "unable to restore PSMDB Cluster")
			return reconcile.Result{}, err
		}
	}
	if dbCR.Spec.Engine.Type == everestv1alpha1.DatabaseEnginePostgresql {
		if err := r.restorePG(ctx, cr); err != nil {
			// The DatabaseCluster controller is responsible for updating the
			// upstream DB cluster with the necessary storage definition. If
			// the storage is not defined in the upstream DB cluster CR, we
			// requeue the backup to give the DatabaseCluster controller a
			// chance to update the upstream DB cluster CR.
			if errors.Is(err, ErrBackupStorageUndefined) {
				return reconcile.Result{Requeue: true}, nil
			}

			logger.Error(err, "unable to restore PG Cluster")
			return reconcile.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

func (r *DatabaseClusterRestoreReconciler) ensureClusterIsReady(ctx context.Context, restore *everestv1alpha1.DatabaseClusterRestore) error {
	timeoutCtx, cancel := context.WithTimeout(ctx, 15*time.Minute)
	defer cancel()
	for {
		select {
		case <-timeoutCtx.Done():
			return errors.New("wait timeout exceeded")
		default:
			cluster := &everestv1alpha1.DatabaseCluster{}
			err := r.Get(ctx, types.NamespacedName{Name: restore.Spec.DBClusterName, Namespace: restore.Namespace}, cluster)
			if err != nil {
				return err
			}
			if cluster.Status.Status == everestv1alpha1.AppStateReady ||
				cluster.Status.Status == everestv1alpha1.AppStateRestoring {
				return nil
			}
		}
	}
}

func (r *DatabaseClusterRestoreReconciler) restorePSMDB(
	ctx context.Context, restore *everestv1alpha1.DatabaseClusterRestore,
) error {
	logger := log.FromContext(ctx)
	if err := r.ensureClusterIsReady(ctx, restore); err != nil {
		return err
	}

	// We need to check if the storage used by the backup is defined in the
	// PerconaServerMongoDB CR. If not, we requeue the restore to give the
	// DatabaseCluster controller a chance to update the PSMDB cluster CR.
	// Otherwise, the restore will fail.
	if restore.Spec.DataSource.DBClusterBackupName != "" {
		backup := &everestv1alpha1.DatabaseClusterBackup{}
		err := r.Get(ctx, types.NamespacedName{Name: restore.Spec.DataSource.DBClusterBackupName, Namespace: restore.Namespace}, backup)
		if err != nil {
			logger.Error(err, "unable to fetch DatabaseClusterBackup")
			return err
		}

		psmdbDBCR := &psmdbv1.PerconaServerMongoDB{}
		err = r.Get(ctx, types.NamespacedName{Name: restore.Spec.DBClusterName, Namespace: restore.Namespace}, psmdbDBCR)
		if err != nil {
			logger.Error(err, "unable to fetch PerconaServerMongoDB")
			return err
		}

		// If the backup storage is not defined in the PerconaServerMongoDB CR,
		// we cannot proceed
		if psmdbDBCR.Spec.Backup.Storages == nil {
			logger.Info(
				fmt.Sprintf("Backup storage %s is not defined in the psmdb cluster %s, requeuing",
					backup.Spec.BackupStorageName,
					restore.Spec.DBClusterName),
			)
			return ErrBackupStorageUndefined
		}
		if _, ok := psmdbDBCR.Spec.Backup.Storages[backup.Spec.BackupStorageName]; !ok {
			logger.Info(
				fmt.Sprintf("Backup storage %s is not defined in the psmdb cluster %s, requeuing",
					backup.Spec.BackupStorageName,
					restore.Spec.DBClusterName),
			)
			return ErrBackupStorageUndefined
		}
	}

	psmdbCR := &psmdbv1.PerconaServerMongoDBRestore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      restore.Name,
			Namespace: restore.Namespace,
		},
	}
	if err := controllerutil.SetControllerReference(restore, psmdbCR, r.Client.Scheme()); err != nil {
		return err
	}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, psmdbCR, func() error {
		psmdbCR.Spec.ClusterName = restore.Spec.DBClusterName
		if restore.Spec.DataSource.DBClusterBackupName != "" {
			psmdbCR.Spec.BackupName = restore.Spec.DataSource.DBClusterBackupName
		}
		if restore.Spec.DataSource.BackupSource != nil {
			backupStorage := &everestv1alpha1.BackupStorage{}
			err := r.Get(ctx, types.NamespacedName{Name: restore.Spec.DataSource.BackupSource.BackupStorageName, Namespace: r.systemNamespace}, backupStorage)
			if err != nil {
				return errors.Join(err, fmt.Errorf("failed to get backup storage %s", restore.Spec.DataSource.BackupSource.BackupStorageName))
			}

			psmdbCR.Spec.BackupSource = &psmdbv1.PerconaServerMongoDBBackupStatus{
				Destination: restore.Spec.DataSource.BackupSource.Path,
			}
			switch backupStorage.Spec.Type {
			case everestv1alpha1.BackupStorageTypeS3:
				psmdbCR.Spec.BackupSource.S3 = &psmdbv1.BackupStorageS3Spec{
					Bucket:            backupStorage.Spec.Bucket,
					CredentialsSecret: backupStorage.Spec.CredentialsSecretName,
					Region:            backupStorage.Spec.Region,
					EndpointURL:       backupStorage.Spec.EndpointURL,
					Prefix:            parsePrefixFromDestination(restore.Spec.DataSource.BackupSource.Path),
				}
			case everestv1alpha1.BackupStorageTypeAzure:
				psmdbCR.Spec.BackupSource.Azure = &psmdbv1.BackupStorageAzureSpec{
					Container:         backupStorage.Spec.Bucket,
					Prefix:            parsePrefixFromDestination(restore.Spec.DataSource.BackupSource.Path),
					CredentialsSecret: backupStorage.Spec.CredentialsSecretName,
				}
			default:
				return fmt.Errorf("unsupported backup storage type %s for %s", backupStorage.Spec.Type, backupStorage.Name)
			}
		}

		if restore.Spec.DataSource.PITR != nil {
			if err := validatePitrRestoreSpec(restore.Spec.DataSource); err != nil {
				return err
			}

			psmdbCR.Spec.PITR = &psmdbv1.PITRestoreSpec{
				Type: psmdbv1.PITRestoreType(restore.Spec.DataSource.PITR.Type),
				Date: &psmdbv1.PITRestoreDate{Time: restore.Spec.DataSource.PITR.Date.Time},
			}
		}

		return nil
	})
	if err != nil {
		return err
	}

	psmdbCR = &psmdbv1.PerconaServerMongoDBRestore{}
	err = r.Get(ctx, types.NamespacedName{Name: restore.Name, Namespace: restore.Namespace}, psmdbCR)
	if err != nil {
		return err
	}

	restore.Status.State = everestv1alpha1.RestoreState(psmdbCR.Status.State)
	restore.Status.CompletedAt = psmdbCR.Status.CompletedAt
	restore.Status.Message = psmdbCR.Status.Error

	return r.Status().Update(ctx, restore)
}

func (r *DatabaseClusterRestoreReconciler) restorePXC(
	ctx context.Context,
	restore *everestv1alpha1.DatabaseClusterRestore,
	db *everestv1alpha1.DatabaseCluster,
) error {
	pxcCR := &pxcv1.PerconaXtraDBClusterRestore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      restore.Name,
			Namespace: restore.Namespace,
		},
	}

	if err := controllerutil.SetControllerReference(restore, pxcCR, r.Client.Scheme()); err != nil {
		return err
	}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, pxcCR, func() error {
		pxcCR.Spec.PXCCluster = restore.Spec.DBClusterName
		if restore.Spec.DataSource.DBClusterBackupName != "" {
			pxcCR.Spec.BackupName = restore.Spec.DataSource.DBClusterBackupName
		}

		dataSource := restore.Spec.DataSource
		if dataSource.BackupSource != nil {
			backupStorage := &everestv1alpha1.BackupStorage{}
			err := r.Get(ctx, types.NamespacedName{Name: dataSource.BackupSource.BackupStorageName, Namespace: r.systemNamespace}, backupStorage)
			if err != nil {
				return errors.Join(err, fmt.Errorf("failed to get backup storage %s", restore.Spec.DataSource.BackupSource.BackupStorageName))
			}

			pxcCR.Spec.BackupSource = &pxcv1.PXCBackupStatus{
				Destination: fmt.Sprintf("s3://%s/%s", backupStorage.Spec.Bucket, dataSource.BackupSource.Path),
				VerifyTLS:   backupStorage.Spec.VerifyTLS,
			}
			switch backupStorage.Spec.Type {
			case everestv1alpha1.BackupStorageTypeS3:
				pxcCR.Spec.BackupSource.S3 = &pxcv1.BackupStorageS3Spec{
					Bucket: fmt.Sprintf(
						"%s/%s",
						backupStorage.Spec.Bucket,
						backupStoragePrefix(db),
					),
					CredentialsSecret: backupStorage.Spec.CredentialsSecretName,
					Region:            backupStorage.Spec.Region,
					EndpointURL:       backupStorage.Spec.EndpointURL,
				}
			case everestv1alpha1.BackupStorageTypeAzure:
				pxcCR.Spec.BackupSource.Azure = &pxcv1.BackupStorageAzureSpec{
					ContainerPath: fmt.Sprintf(
						"%s/%s",
						backupStorage.Spec.Bucket,
						backupStoragePrefix(db),
					),
					CredentialsSecret: backupStorage.Spec.CredentialsSecretName,
				}
			default:
				return fmt.Errorf("unsupported backup storage type %s for %s", backupStorage.Spec.Type, backupStorage.Name)
			}
		}
		if dataSource.PITR != nil {
			spec, err := r.genPXCPitrRestoreSpec(ctx, dataSource, *db)
			if err != nil {
				return err
			}
			pxcCR.Spec.PITR = spec
		}
		return nil
	})
	if err != nil {
		return err
	}

	pxcCR = &pxcv1.PerconaXtraDBClusterRestore{}
	err = r.Get(ctx, types.NamespacedName{Name: restore.Name, Namespace: restore.Namespace}, pxcCR)
	if err != nil {
		return err
	}

	restore.Status.State = everestv1alpha1.RestoreState(pxcCR.Status.State)
	restore.Status.CompletedAt = pxcCR.Status.CompletedAt
	restore.Status.Message = pxcCR.Status.Comments

	return r.Status().Update(ctx, restore)
}

func (r *DatabaseClusterRestoreReconciler) restorePG(ctx context.Context, restore *everestv1alpha1.DatabaseClusterRestore) error {
	logger := log.FromContext(ctx)

	var backupStorageName string
	var backupBaseName string
	if restore.Spec.DataSource.DBClusterBackupName != "" {
		backup := &everestv1alpha1.DatabaseClusterBackup{}
		err := r.Get(ctx, types.NamespacedName{Name: restore.Spec.DataSource.DBClusterBackupName, Namespace: restore.Namespace}, backup)
		if err != nil {
			logger.Error(err, "unable to fetch DatabaseClusterBackup")
			return err
		}

		backupStorageName = backup.Spec.BackupStorageName
		if backup.Status.Destination != nil {
			backupBaseName = filepath.Base(*backup.Status.Destination)
		}
	}
	if restore.Spec.DataSource.BackupSource != nil {
		backupStorageName = restore.Spec.DataSource.BackupSource.BackupStorageName
		backupBaseName = filepath.Base(restore.Spec.DataSource.BackupSource.Path)
	}

	pgDBCR := &pgv2.PerconaPGCluster{}
	err := r.Get(ctx, types.NamespacedName{Name: restore.Spec.DBClusterName, Namespace: restore.Namespace}, pgDBCR)
	if err != nil {
		logger.Error(err, "unable to fetch PerconaPGCluster")
		return err
	}

	backupStorage := &everestv1alpha1.BackupStorage{}
	err = r.Get(ctx, types.NamespacedName{Name: backupStorageName, Namespace: r.systemNamespace}, backupStorage)
	if err != nil {
		return errors.Join(err, fmt.Errorf("failed to get backup storage %s", restore.Spec.DataSource.BackupSource.BackupStorageName))
	}

	// We need to check if the storage used by the backup is defined in the
	// PerconaPGCluster CR. If not, we requeue the restore to give the
	// DatabaseCluster controller a chance to update the PG cluster CR.
	// Otherwise, the restore will fail.
	repoIdx := getBackupStorageIndexInPGBackrestRepo(backupStorage, pgDBCR.Spec.Backups.PGBackRest.Repos)
	if repoIdx == -1 {
		logger.Info(
			fmt.Sprintf("Backup storage %s is not defined in the pg cluster %s, requeuing",
				backupStorageName,
				restore.Spec.DBClusterName),
		)
		return ErrBackupStorageUndefined
	}

	pgCR := &pgv2.PerconaPGRestore{
		ObjectMeta: metav1.ObjectMeta{
			Name:      restore.Name,
			Namespace: restore.Namespace,
		},
	}
	if err := controllerutil.SetControllerReference(restore, pgCR, r.Client.Scheme()); err != nil {
		return err
	}
	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, pgCR, func() error {
		pgCR.Spec.PGCluster = restore.Spec.DBClusterName
		pgCR.Spec.RepoName = pgDBCR.Spec.Backups.PGBackRest.Repos[repoIdx].Name
		pgCR.Spec.Options, err = getPGRestoreOptions(restore.Spec.DataSource, backupBaseName)
		return err
	})
	if err != nil {
		return err
	}

	restore.Status.State = everestv1alpha1.RestoreState(pgCR.Status.State)
	restore.Status.CompletedAt = pgCR.Status.CompletedAt

	return r.Status().Update(ctx, restore)
}

func (r *DatabaseClusterRestoreReconciler) addPXCKnownTypes(scheme *runtime.Scheme) error {
	pxcSchemeGroupVersion := schema.GroupVersion{Group: "pxc.percona.com", Version: "v1"}
	scheme.AddKnownTypes(pxcSchemeGroupVersion,
		&pxcv1.PerconaXtraDBClusterRestore{}, &pxcv1.PerconaXtraDBClusterRestoreList{})

	metav1.AddToGroupVersion(scheme, pxcSchemeGroupVersion)
	return nil
}

func (r *DatabaseClusterRestoreReconciler) addPSMDBKnownTypes(scheme *runtime.Scheme) error {
	psmdbSchemeGroupVersion := schema.GroupVersion{Group: "psmdb.percona.com", Version: "v1"}
	scheme.AddKnownTypes(psmdbSchemeGroupVersion,
		&psmdbv1.PerconaServerMongoDBRestore{}, &psmdbv1.PerconaServerMongoDBRestoreList{})

	metav1.AddToGroupVersion(scheme, psmdbSchemeGroupVersion)
	return nil
}

func (r *DatabaseClusterRestoreReconciler) addPGKnownTypes(scheme *runtime.Scheme) error {
	pgSchemeGroupVersion := schema.GroupVersion{Group: "pgv2.percona.com", Version: "v2"}
	scheme.AddKnownTypes(pgSchemeGroupVersion,
		&pgv2.PerconaPGRestore{}, &pgv2.PerconaPGRestoreList{})

	metav1.AddToGroupVersion(scheme, pgSchemeGroupVersion)
	return nil
}

func (r *DatabaseClusterRestoreReconciler) addPXCToScheme(scheme *runtime.Scheme) error {
	builder := runtime.NewSchemeBuilder(r.addPXCKnownTypes)
	return builder.AddToScheme(scheme)
}

func (r *DatabaseClusterRestoreReconciler) addPSMDBToScheme(scheme *runtime.Scheme) error {
	builder := runtime.NewSchemeBuilder(r.addPSMDBKnownTypes)
	return builder.AddToScheme(scheme)
}

func (r *DatabaseClusterRestoreReconciler) addPGToScheme(scheme *runtime.Scheme) error {
	builder := runtime.NewSchemeBuilder(r.addPGKnownTypes)
	return builder.AddToScheme(scheme)
}

// SetupWithManager sets up the controller with the Manager.
func (r *DatabaseClusterRestoreReconciler) SetupWithManager(mgr ctrl.Manager, systemNamespace string) error {
	unstructuredResource := &unstructured.Unstructured{}
	unstructuredResource.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "apiextensions.k8s.io",
		Kind:    "CustomResourceDefinition",
		Version: "v1",
	})
	controller := ctrl.NewControllerManagedBy(mgr).
		For(&everestv1alpha1.DatabaseClusterRestore{})
	err := r.Get(context.Background(), types.NamespacedName{Name: pxcRestoreCRDName}, unstructuredResource)
	if err == nil {
		if err := r.addPXCToScheme(r.Scheme); err == nil {
			controller.Owns(&pxcv1.PerconaXtraDBClusterRestore{})
		}
	}
	err = r.Get(context.Background(), types.NamespacedName{Name: psmdbRestoreCRDName}, unstructuredResource)
	if err == nil {
		if err := r.addPSMDBToScheme(r.Scheme); err == nil {
			controller.Owns(&psmdbv1.PerconaServerMongoDBRestore{})
		}
	}
	err = r.Get(context.Background(), types.NamespacedName{Name: pgRestoreCRDName}, unstructuredResource)
	if err == nil {
		if err := r.addPGToScheme(r.Scheme); err == nil {
			controller.Owns(&pgv2.PerconaPGRestore{})
		}
	}
	err = mgr.GetFieldIndexer().IndexField(
		context.Background(),
		&everestv1alpha1.DatabaseClusterRestore{},
		dbClusterRestoreDBClusterNameField,
		func(rawObj client.Object) []string {
			res := rawObj.(*everestv1alpha1.DatabaseClusterRestore) //nolint:forcetypeassert
			return []string{res.Spec.DBClusterName}
		},
	)
	if err != nil {
		return err
	}

	r.systemNamespace = systemNamespace

	return controller.Complete(r)
}

func parsePrefixFromDestination(url string) string {
	parts := strings.Split(url, "/")
	l := len(parts)
	// taking the third and the second last parts of the destination
	return fmt.Sprintf("%s/%s", parts[l-3], parts[l-2])
}

func (r *DatabaseClusterRestoreReconciler) genPXCPitrRestoreSpec(
	ctx context.Context,
	dataSource everestv1alpha1.DataSource,
	db everestv1alpha1.DatabaseCluster,
) (*pxcv1.PITR, error) {
	if db.Spec.Backup.PITR.BackupStorageName == nil || *db.Spec.Backup.PITR.BackupStorageName == "" {
		return nil, fmt.Errorf("no backup storage defined for PITR in %s cluster", db.Name)
	}
	// use 'date' as default
	if dataSource.PITR.Type == "" {
		dataSource.PITR.Type = everestv1alpha1.PITRTypeDate
	}

	if err := validatePitrRestoreSpec(dataSource); err != nil {
		return nil, err
	}

	// First get the source backup object.
	// Note: This assumes that we will always restore to same namespace, even to a new cluster.
	sourceBackup := &everestv1alpha1.DatabaseClusterBackup{}
	key := types.NamespacedName{Name: dataSource.DBClusterBackupName, Namespace: db.GetNamespace()}
	if err := r.Get(ctx, key, sourceBackup); err != nil {
		return nil, fmt.Errorf("failed to get source backup %s: %w", dataSource.DBClusterBackupName, err)
	}
	// Get the source cluster the backup belongs to.
	sourceDB := &everestv1alpha1.DatabaseCluster{}
	key = types.NamespacedName{Name: sourceBackup.Spec.DBClusterName, Namespace: sourceBackup.GetNamespace()}
	if err := r.Get(ctx, key, sourceDB); err != nil {
		return nil, fmt.Errorf("failed to get source cluster for backup %s: %w", dataSource.DBClusterBackupName, err)
	}
	// Get the storage object where the source backup is stored.
	backupStorage := &everestv1alpha1.BackupStorage{}
	key = types.NamespacedName{Name: sourceBackup.Spec.BackupStorageName, Namespace: r.systemNamespace}
	if err := r.Get(ctx, key, backupStorage); err != nil {
		return nil, fmt.Errorf("failed to get backup storage '%s' for backup: %w", sourceBackup.Spec.BackupStorageName, err)
	}

	spec := &pxcv1.PITR{
		BackupSource: &pxcv1.PXCBackupStatus{},
		Type:         string(dataSource.PITR.Type),
		Date:         dataSource.PITR.Date.Format(everestv1alpha1.DateFormatSpace),
	}

	switch backupStorage.Spec.Type {
	case everestv1alpha1.BackupStorageTypeS3:
		spec.BackupSource.S3 = &pxcv1.BackupStorageS3Spec{
			CredentialsSecret: backupStorage.Spec.CredentialsSecretName,
			Region:            backupStorage.Spec.Region,
			EndpointURL:       backupStorage.Spec.EndpointURL,
			Bucket:            pitrBucketName(sourceDB, backupStorage.Spec.Bucket),
		}
		//nolint:godox
		// TODO: add support for Azure.
	default:
		return nil, fmt.Errorf("unsupported backup storage type %s for %s", backupStorage.Spec.Type, backupStorage.Name)
	}
	return spec, nil
}

func getPGRestoreOptions(dataSource everestv1alpha1.DataSource, backupBaseName string) ([]string, error) {
	options := []string{
		"--set=" + backupBaseName,
	}

	if dataSource.PITR != nil {
		if err := validatePitrRestoreSpec(dataSource); err != nil {
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

func validatePitrRestoreSpec(dataSource everestv1alpha1.DataSource) error {
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
			return errPitrEmptyDate
		}
	case everestv1alpha1.PITRTypeLatest:
		//nolint:godox
		// TODO: figure out why "latest" doesn't work currently for Everest
		return errPitrTypeLatest
	default:
		return errPitrTypeIsNotSupported
	}
	return nil
}
