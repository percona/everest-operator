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
package webhooks

import (
	"context"
	"encoding/base64"
	"fmt"

	everestv1alpha1 "github.com/percona/everest-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// SetupDataImportJobWebhookWithManager sets up the webhook for DataImportJob.
func SetupDataImportJobWebhookWithManager(mgr manager.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&everestv1alpha1.DataImportJob{}).
		WithDefaulter(&DataImportJobDefaulter{
			Client: mgr.GetClient(),
		}).
		Complete()
}

// +kubebuilder:webhook:path=/mutate-everest-percona-com-v1alpha1-dataimportjob,mutating=true,failurePolicy=fail,sideEffects=None,groups=everest.percona.com,resources=dataimportjobs,verbs=create;update,versions=v1alpha1,name=mdataimportjobs-v1akpha1.everest.percona.com,admissionReviewVersions=v1
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=create;update;get;list
// +kubebuilder:rbac:groups=everest.percona.com,resources=dataimportjobs,verbs=get;list;watch;update

// DataImportJobDefaulter is a webhook that sets default values for DataImportJob resources.
type DataImportJobDefaulter struct {
	Client client.Client
}

func (d *DataImportJobDefaulter) Default(ctx context.Context, obj runtime.Object) error {
	dij, ok := obj.(*everestv1alpha1.DataImportJob)

	if !ok {
		return fmt.Errorf("expected an DataImportJob object but got %T", obj)
	}
	return handleS3CredentialsSecret(ctx, d.Client, dij.GetNamespace(), dij.Spec.DataImportJobTemplate)
}

func handleS3CredentialsSecret(
	ctx context.Context,
	c client.Client,
	namespace string,
	tpl *everestv1alpha1.DataImportJobTemplate,
) error {
	logger.Info("called?")
	if tpl == nil || tpl.Source == nil || tpl.Source.S3 == nil {
		return nil
	}
	accessKeyID := tpl.Source.S3.AccessKeyID
	secretAccessKey := tpl.Source.S3.SecretAccessKey

	logger.Info("Handling S3 credentials secret", "accessKeyID", accessKeyID, "secretAccessKey", secretAccessKey)

	switch {
	case accessKeyID != "" && secretAccessKey == "":
		return fmt.Errorf("secretAccessKey is not provided")
	case accessKeyID == "" && secretAccessKey != "":
		return fmt.Errorf("accessKeyID is not provided")
	case accessKeyID == "" && secretAccessKey == "":
		return nil // nothing to do for us
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      tpl.Source.S3.CredentialsSecretName,
			Namespace: namespace,
		},
	}
	if _, err := controllerutil.CreateOrUpdate(ctx, c, secret, func() error {
		return mutateS3CredentialsSecret(secret, accessKeyID, secretAccessKey)
	}); err != nil {
		return fmt.Errorf("failed to create or update S3 credentials secret: %w", err)
	}

	// remove from object
	tpl.Source.S3.AccessKeyID = ""
	tpl.Source.S3.SecretAccessKey = ""
	return nil
}

const (
	AccessKeyIDSecretKey     = "AWS_ACCESS_KEY_ID"
	SecretAccessKeySecretKey = "AWS_SECRET_ACCESS_KEY"
)

func mutateS3CredentialsSecret(
	secret *corev1.Secret,
	accessKeyID, secretAccessKey string,
) error {
	switch {
	case isBase64Encoded(accessKeyID) && isBase64Encoded(secretAccessKey):
		secret.Data = map[string][]byte{
			AccessKeyIDSecretKey:     []byte(accessKeyID),
			SecretAccessKeySecretKey: []byte(secretAccessKey),
		}
	case !isBase64Encoded(accessKeyID) && !isBase64Encoded(secretAccessKey):
		secret.StringData = map[string]string{
			AccessKeyIDSecretKey:     accessKeyID,
			SecretAccessKeySecretKey: secretAccessKey,
		}
	default:
		return fmt.Errorf("both accessKeyID and secretAccessKey must be either base64 encoded or not")
	}
	return nil
}

func isBase64Encoded(s string) bool {
	_, err := base64.StdEncoding.DecodeString(s)
	return len(s)%4 == 0 && len(s) > 0 && err == nil
}
