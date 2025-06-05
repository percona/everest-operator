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
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/AlekSi/pointer"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	everestv1alpha1 "github.com/percona/everest-operator/api/v1alpha1"
	"github.com/percona/everest-operator/api/v1alpha1/dataimporterspec"
)

const (
	dataImporterRequestSecretNameSuffix = "-data-import-request"
	dataImportJSONSecretKey             = "request.json"
)

type DataImportJobReconciler struct {
	Client client.Client
	Scheme *runtime.Scheme
}

func (r *DataImportJobReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named("DataImportJob").
		For(&everestv1alpha1.DataImportJob{}).
		Owns(&batchv1.Job{}).
		Owns(&corev1.Secret{}).
		Complete(r)
}

//+kubebuilder:rbac:groups=everest.percona.com,resources=dataimportjobs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=everest.percona.com,resources=dataimportjobs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=everest.percona.com,resources=dataimportjobs/finalizers,verbs=update
//+kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=everest.percona.com,resources=dataimporters,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.1/pkg/reconcile
func (r *DataImportJobReconciler) Reconcile(
	ctx context.Context,
	req ctrl.Request,
) (rr ctrl.Result, rerr error) {
	diJob := &everestv1alpha1.DataImportJob{}
	if err := r.Client.Get(ctx, req.NamespacedName, diJob); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	l := log.FromContext(ctx)
	l.Info("Reconciling", "name", diJob.GetName())

	if diJob.Status.Phase == everestv1alpha1.DataImportJobPhaseCompleted {
		// Already complete, no need to reconcile again.
		return ctrl.Result{}, nil
	}

	// Reset the status, we will build a new one by observing the current state on each reconcile.
	startedAt := diJob.Status.StartedAt
	diJob.Status = everestv1alpha1.DataImportJobStatus{}
	diJob.Status.LastObservedGeneration = diJob.GetGeneration()
	if startedAt != nil && !startedAt.Time.IsZero() {
		diJob.Status.StartedAt = startedAt
	}

	// Sync status on finishing reconciliation.
	defer func() {
		if updErr := r.Client.Status().Update(ctx, diJob); updErr != nil {
			l.Error(updErr, "Failed to update data import job status")
			rerr = errors.Join(rerr, updErr)
		}
	}()

	// Get the referenced data importer.
	di := &everestv1alpha1.DataImporter{}
	if err := r.Client.Get(ctx, client.ObjectKey{
		Name: diJob.Spec.DataImporterName,
	}, di); err != nil {
		diJob.Status.Phase = everestv1alpha1.DataImportJobPhaseError
		diJob.Status.Message = err.Error()
		return ctrl.Result{}, err
	}

	// Get the target database cluster.
	db := &everestv1alpha1.DatabaseCluster{}
	if err := r.Client.Get(ctx, client.ObjectKey{
		Name:      diJob.Spec.TargetClusterName,
		Namespace: diJob.GetNamespace(),
	}, db); err != nil {
		diJob.Status.Phase = everestv1alpha1.DataImportJobPhaseError
		diJob.Status.Message = fmt.Errorf("failed to get database cluster: %w", err).Error()
		return ctrl.Result{}, err
	}

	// Create payload secret.
	if err := r.ensureDataImportPayloadSecret(ctx, diJob, di, db); err != nil {
		diJob.Status.Phase = everestv1alpha1.DataImportJobPhaseError
		diJob.Status.Message = fmt.Errorf("failed to create data import payload secret: %w", err).Error()
		return ctrl.Result{}, err
	}

	// Create import job.
	if err := r.ensureImportJob(ctx, diJob, di); err != nil {
		diJob.Status.Phase = everestv1alpha1.DataImportJobPhaseError
		diJob.Status.Message = fmt.Errorf("failed to create import job: %w", err).Error()
		return ctrl.Result{}, err
	}

	// Observe import state.
	if err := r.observeImportState(ctx, diJob); err != nil {
		diJob.Status.Phase = everestv1alpha1.DataImportJobPhaseError
		diJob.Status.Message = fmt.Errorf("failed to observe state: %w", err).Error()
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *DataImportJobReconciler) observeImportState(ctx context.Context, diJob *everestv1alpha1.DataImportJob) error {
	jobName := diJob.Status.JobName
	if jobName == "" {
		diJob.Status.Phase = everestv1alpha1.DataImportJobPhasePending
		return nil
	}

	job := &batchv1.Job{}
	if err := r.Client.Get(ctx, client.ObjectKey{
		Name:      jobName,
		Namespace: diJob.GetNamespace(),
	}, job); err != nil {
		return fmt.Errorf("failed to get import job: %w", err)
	}

	for _, c := range job.Status.Conditions {
		if c.Type == batchv1.JobComplete && c.Status == corev1.ConditionTrue {
			// Job is complete, delete the secret.
			if err := r.Client.Delete(ctx, &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      dataImporterRequestSecretName(diJob),
					Namespace: diJob.GetNamespace(),
				},
			}); err != nil {
				return fmt.Errorf("failed to delete data import request secret: %w", err)
			}
			diJob.Status.Phase = everestv1alpha1.DataImportJobPhaseCompleted
			return nil
		}

		if c.Type == batchv1.JobFailed && c.Status == corev1.ConditionTrue {
			diJob.Status.Phase = everestv1alpha1.DataImportJobPhaseFailed
			diJob.Status.Message = c.Message
			return nil
		}
	}
	diJob.Status.Phase = everestv1alpha1.DataImportJobPhaseRunning
	return nil
}

func (r *DataImportJobReconciler) ensureDataImportPayloadSecret(
	ctx context.Context,
	diJob *everestv1alpha1.DataImportJob,
	di *everestv1alpha1.DataImporter,
	db *everestv1alpha1.DatabaseCluster,
) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dataImporterRequestSecretName(diJob),
			Namespace: diJob.GetNamespace(),
		},
	}

	req := dataimporterspec.Spec{}

	dbUser, dbPassword, err := r.getDBRootUserCredentials(ctx, db)
	if err != nil {
		return fmt.Errorf("failed to get database root user credentials: %w", err)
	}
	req.Target = dataimporterspec.Target{
		User:     dbUser,
		Password: dbPassword,
		Host:     db.Status.Hostname,
		Port:     strconv.Itoa(int(db.Status.Port)),
		Type:     string(db.Spec.Engine.Type),
		DatabaseClusterRef: &dataimporterspec.ObjectReference{
			Name:      db.GetName(),
			Namespace: db.GetNamespace(),
		},
	}

	// Get S3 info
	s3Info, err := r.getS3Info(ctx, diJob)
	if err != nil {
		return fmt.Errorf("failed to get S3 info: %w", err)
	}
	req.Source.S3 = s3Info
	req.Source.Path = diJob.Spec.Source.Path

	cfgMap := map[string]any{}

	if cfg := diJob.Spec.Config; cfg != nil {
		if err := json.Unmarshal(cfg.Raw, &cfgMap); err != nil {
			return fmt.Errorf("failed to unmarshal data import job parameters: %w", err)
		}
	}
	req.Config = cfgMap

	reqJSON, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("failed to marshal data import payload: %w", err)
	}
	if _, err := ctrl.CreateOrUpdate(ctx, r.Client, secret, func() error {
		secret.Data = map[string][]byte{
			dataImportJSONSecretKey: reqJSON,
		}
		if err := controllerutil.SetControllerReference(diJob, secret, r.Scheme); err != nil {
			return fmt.Errorf("failed to set controller reference: %w", err)
		}
		return nil
	}); err != nil {
		return fmt.Errorf("failed to create or update data import request secret: %w", err)
	}
	return nil
}

func (r *DataImportJobReconciler) getS3Info(
	ctx context.Context,
	dij *everestv1alpha1.DataImportJob,
) (*dataimporterspec.S3, error) {
	if dij.Spec.Source.S3 == nil {
		return nil, errors.New("s3 info not provided")
	}
	info := dataimporterspec.S3{}

	keyId, keySecret, err := r.getS3Credentials(ctx, dij)
	if err != nil {
		return nil, fmt.Errorf("failed to get S3 credentials: %w", err)
	}
	info.AccessKeyID = keyId
	info.SecretKey = keySecret
	info.Bucket = dij.Spec.Source.S3.Bucket
	info.Region = dij.Spec.Source.S3.Region
	info.EndpointURL = dij.Spec.Source.S3.EndpointURL
	info.VerifyTLS = pointer.Get(dij.Spec.Source.S3.VerifyTLS)
	info.ForcePathStyle = pointer.Get(dij.Spec.Source.S3.ForcePathStyle)
	return &info, nil
}

func (r *DataImportJobReconciler) getS3Credentials(
	ctx context.Context,
	dij *everestv1alpha1.DataImportJob,
) (string, string, error) {
	namespace := dij.GetNamespace()
	secretName := dij.Spec.Source.S3.CredentialsSecretName
	secret := &corev1.Secret{}
	if err := r.Client.Get(ctx, client.ObjectKey{
		Name:      secretName,
		Namespace: namespace,
	}, secret); err != nil {
		return "", "", err
	}

	// Take ownership of the secret
	if err := controllerutil.SetControllerReference(dij, secret, r.Scheme); err != nil &&
		!errors.Is(err, &controllerutil.AlreadyOwnedError{}) {
		return "", "", fmt.Errorf("failed to set controller reference for secret %s: %w", secretName, err)
	}
	if err := r.Client.Update(ctx, secret); err != nil {
		return "", "", fmt.Errorf("failed to update secret %s: %w", secretName, err)
	}

	accessKey := secret.Data["AWS_ACCESS_KEY_ID"]
	secretKey := secret.Data["AWS_SECRET_ACCESS_KEY"]
	return string(accessKey), string(secretKey), nil
}

func (r *DataImportJobReconciler) getDBRootUserCredentials(
	ctx context.Context,
	db *everestv1alpha1.DatabaseCluster,
) (string, string, error) {
	secret := &corev1.Secret{}
	if err := r.Client.Get(ctx, client.ObjectKey{
		Name:      db.Spec.Engine.UserSecretsName,
		Namespace: db.GetNamespace(),
	}, secret); err != nil {
		return "", "", err
	}

	switch db.Spec.Engine.Type {
	case everestv1alpha1.DatabaseEnginePostgresql:
		username := "postgres"
		pwd, ok := secret.Data["password"]
		if !ok {
			return "", "", fmt.Errorf("postgres password not found in secret")
		}
		return username, string(pwd), nil
	case everestv1alpha1.DatabaseEnginePXC:
		username := "root"
		pwd, ok := secret.Data["root"]
		if !ok {
			return "", "", fmt.Errorf("root password not found in secret")
		}
		return username, string(pwd), nil
	case everestv1alpha1.DatabaseEnginePSMDB:
		username, ok := secret.Data["MONGODB_DATABASE_ADMIN_USER"]
		if !ok {
			return "", "", fmt.Errorf("mongodb admin user not found in secret")
		}
		pwd, ok := secret.Data["MONGODB_DATABASE_ADMIN_PASSWORD"]
		if !ok {
			return "", "", fmt.Errorf("mongodb admin password not found in secret")
		}
		return string(username), string(pwd), nil
	default:
		return "", "", fmt.Errorf("unsupported database engine type: %s", db.Spec.Engine.Type)
	}
}

func (r *DataImportJobReconciler) ensureImportJob(
	ctx context.Context,
	diJob *everestv1alpha1.DataImportJob,
	di *everestv1alpha1.DataImporter,
) error {
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dataImporterJobName(diJob),
			Namespace: diJob.GetNamespace(),
		},
	}

	defer func() {
		diJob.Status.JobName = job.GetName()
	}()

	// Check if the job already exists.
	if err := r.Client.Get(ctx, client.ObjectKeyFromObject(job), job); err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to get import job: %w", err)
		}
	} else if err == nil {
		return nil
	}

	job.Spec = r.getJobSpec(diJob, di)
	if err := controllerutil.SetControllerReference(diJob, job, r.Scheme); err != nil {
		return fmt.Errorf("failed to set controller reference: %w", err)
	}
	job.Status.StartTime = pointer.To(metav1.Now())
	return r.Client.Create(ctx, job)
}

func dataImporterJobName(diJob *everestv1alpha1.DataImportJob) string {
	uuid := diJob.GetUID()
	hash := md5.Sum([]byte(uuid))
	hashStr := hex.EncodeToString(hash[:])
	return fmt.Sprintf("%s-%s", diJob.GetName(), hashStr[:6])
}

func dataImporterRequestSecretName(diJob *everestv1alpha1.DataImportJob) string {
	return dataImporterJobName(diJob) + dataImporterRequestSecretNameSuffix
}

func (r *DataImportJobReconciler) getJobSpec(
	diJob *everestv1alpha1.DataImportJob,
	di *everestv1alpha1.DataImporter,
) batchv1.JobSpec {
	spec := batchv1.JobSpec{
		Template: corev1.PodTemplateSpec{
			Spec: corev1.PodSpec{
				RestartPolicy:      corev1.RestartPolicyNever,
				ServiceAccountName: di.Spec.JobSpec.ServiceAccountName,
				Containers: []corev1.Container{{
					Name:    "importer",
					Image:   di.Spec.JobSpec.Image,
					Command: di.Spec.JobSpec.Command,
					Args:    []string{fmt.Sprintf("/payload/%s", dataImportJSONSecretKey)},
					VolumeMounts: []corev1.VolumeMount{
						{
							Name:      "payload",
							MountPath: "/payload",
							ReadOnly:  true,
						},
					},
				}},
				Volumes: []corev1.Volume{
					{
						Name: "payload",
						VolumeSource: corev1.VolumeSource{
							Secret: &corev1.SecretVolumeSource{
								SecretName: dataImporterRequestSecretName(diJob),
							},
						},
					},
				},
			},
		},
	}
	return spec
}
