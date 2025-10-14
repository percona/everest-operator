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

// Package v1alpha1 contains a set of WebHooks for the engine-features.everest.percona.com API group
package v1alpha1

import (
	"context"
	"errors"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	enginefeatureseverestv1alpha1 "github.com/percona/everest-operator/api/engine-features.everest/v1alpha1"
	"github.com/percona/everest-operator/internal/consts"
	"github.com/percona/everest-operator/utils"
)

// errors for SplitHorizonDNSConfig webhook validation.
var (
	// .spec.baseDomainNameSuffix errors.
	baseDomainNameSuffixPath       = field.NewPath("spec", "baseDomainNameSuffix")
	errInvalidBaseDomainNameSuffix = func(bdns string, errs []string) *field.Error {
		return field.Invalid(baseDomainNameSuffixPath, bdns, strings.Join(errs, ", "))
	}
	// .spec.tls.secretName errors.
	secretNamePath = field.NewPath("spec", "tls", "secretName")

	// .spec.tls.certificate.
	certificatePath = field.NewPath("spec", "tls", "certificate")
	// .spec.tls.certificate.caCertFile.
	caCertFilePath = certificatePath.Child("caCertFile")
	// .spec.tls.certificate.certFile.
	certFilePath = certificatePath.Child("certFile")
	// .spec.tls.certificate.keyFile.
	keyFilePath = certificatePath.Child("keyFile")

	// Required field error generator.
	errRequiredField = func(fieldPath *field.Path) *field.Error {
		return field.Required(fieldPath, "can not be empty")
	}
	// Base64 encoding error generator.
	errCertWrongEncodingField = func(fieldPath *field.Path, fieldValue string) *field.Error {
		return field.Invalid(fieldPath, fieldValue, "is not base64-encoded")
	}
	// Immutable field error generator.
	errImmutableField = func(fieldPath *field.Path) *field.Error {
		return field.Forbidden(fieldPath, "is immutable and cannot be changed")
	}

	// Deletion errors.
	errDeleteInUse = errors.New("is used by some DB cluster and cannot be deleted")
)

var (
	_         webhook.CustomValidator = &SplitHorizonDNSConfigCustomValidator{}
	groupKind                         = enginefeatureseverestv1alpha1.GroupVersion.WithKind(consts.SplitHorizonDNSConfigKind).GroupKind()
)

// SetupSplitHorizonDNSConfigWebhookWithManager registers the webhook for SplitHorizonDNSConfig in the manager.
func SetupSplitHorizonDNSConfigWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&enginefeatureseverestv1alpha1.SplitHorizonDNSConfig{}).
		WithValidator(&SplitHorizonDNSConfigCustomValidator{
			Client: mgr.GetClient(),
		}).
		Complete()
}

// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
//nolint:lll
// +kubebuilder:webhook:path=/validate-engine-features-everest-percona-com-v1alpha1-splithorizondnsconfig,mutating=false,failurePolicy=fail,sideEffects=None,groups=engine-features.everest.percona.com,resources=splithorizondnsconfigs,verbs=create;update;delete,versions=v1alpha1,name=vsplithorizondnsconfig-v1alpha1.kb.io,admissionReviewVersions=v1

// SplitHorizonDNSConfigCustomValidator struct is responsible for validating the SplitHorizonDNSConfig resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type SplitHorizonDNSConfigCustomValidator struct {
	Client client.Client
}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type SplitHorizonDNSConfig.
func (v *SplitHorizonDNSConfigCustomValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	var allErrs field.ErrorList
	shdc, ok := obj.(*enginefeatureseverestv1alpha1.SplitHorizonDNSConfig)
	if !ok {
		return nil, fmt.Errorf("expected a SplitHorizonDNSConfig object but got %T", obj)
	}

	logger := logf.FromContext(ctx).WithName("SplitHorizonDNSConfigValidator").WithValues(
		"name", shdc.GetName(),
		"namespace", shdc.GetNamespace(),
	)

	logger.Info("Validation for SplitHorizonDNSConfig upon creation")

	// TODO: validate baseDomainNameSuffix length.
	if shdc.Spec.BaseDomainNameSuffix == "" {
		allErrs = append(allErrs, errRequiredField(baseDomainNameSuffixPath))
	} else if errs := utils.ValidateDNSName(shdc.Spec.BaseDomainNameSuffix); errs != nil {
		allErrs = append(allErrs, errInvalidBaseDomainNameSuffix(shdc.Spec.BaseDomainNameSuffix, errs))
	}

	secretName := shdc.Spec.TLS.SecretName
	if secretName == "" {
		allErrs = append(allErrs, errRequiredField(secretNamePath))
		return nil, apierrors.NewInvalid(groupKind, shdc.GetName(), allErrs)
	}

	if shdc.Spec.TLS.Certificate == nil {
		// .spec.tls.certificate is not provided, so .spec.tls.secretName must be used instead.
		// Check that provided secret exists and valid.
		if errs := validateSecret(ctx, v.Client, shdc.GetNamespace(), secretName); errs != nil {
			allErrs = append(allErrs, errs...)
		}
	} else {
		// Both Certificate and SecretName are provided.
		// Certificate fields must be non-empty.
		// Secret is not checked, as Certificate is provided.
		if errs := validateCertificate(shdc.Spec.TLS.Certificate); errs != nil {
			allErrs = append(allErrs, errs...)
		}
	}

	if len(allErrs) == 0 {
		return nil, nil
	}
	return nil, apierrors.NewInvalid(groupKind, shdc.GetName(), allErrs)
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type SplitHorizonDNSConfig.
func (v *SplitHorizonDNSConfigCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	var allErrs field.ErrorList
	oldShdc, ok := oldObj.(*enginefeatureseverestv1alpha1.SplitHorizonDNSConfig)
	if !ok {
		return nil, fmt.Errorf("expected a SplitHorizonDNSConfig object for the newObj but got %T", newObj)
	}

	newShdc, ok := newObj.(*enginefeatureseverestv1alpha1.SplitHorizonDNSConfig)
	if !ok {
		return nil, fmt.Errorf("expected a SplitHorizonDNSConfig object for the newObj but got %T", newObj)
	}

	logger := logf.FromContext(ctx).WithName("SplitHorizonDNSConfigValidator").WithValues(
		"name", oldShdc.GetName(),
		"namespace", oldShdc.GetNamespace(),
	)

	logger.Info("Validation for SplitHorizonDNSConfig upon update")

	if !newShdc.DeletionTimestamp.IsZero() {
		return nil, nil
	}

	// TODO: PSMDB supports such update. Need to handle it later.
	if newShdc.Spec.BaseDomainNameSuffix != oldShdc.Spec.BaseDomainNameSuffix {
		allErrs = append(allErrs, errImmutableField(baseDomainNameSuffixPath))
	}

	if newShdc.Spec.TLS.SecretName != oldShdc.Spec.TLS.SecretName {
		allErrs = append(allErrs, errImmutableField(secretNamePath))
	}

	// Only .spec.tls.certificate can be updated.
	// If it is provided, its fields must be non-empty and properly encoded (base64).
	if errs := validateCertificate(newShdc.Spec.TLS.Certificate); errs != nil {
		allErrs = append(allErrs, errs...)
	}

	if len(allErrs) == 0 {
		return nil, nil
	}

	return nil, apierrors.NewInvalid(groupKind, oldShdc.GetName(), allErrs)
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type SplitHorizonDNSConfig.
func (v *SplitHorizonDNSConfigCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	shdc, ok := obj.(*enginefeatureseverestv1alpha1.SplitHorizonDNSConfig)
	if !ok {
		return nil, fmt.Errorf("expected a SplitHorizonDNSConfig object but got %T", obj)
	}
	logger := logf.FromContext(ctx).WithName("SplitHorizonDNSConfigValidator").WithValues(
		"name", shdc.GetName(),
		"namespace", shdc.GetNamespace(),
	)

	logger.Info("Validation for SplitHorizonDNSConfig upon deletion")

	if !shdc.DeletionTimestamp.IsZero() {
		return nil, nil
	}

	// we should prevent deletion if it is currently in use.
	if utils.IsEverestObjectInUse(shdc) {
		return nil, apierrors.NewForbidden(
			enginefeatureseverestv1alpha1.GroupVersion.WithResource("splithorizondnsconfig").GroupResource(),
			shdc.GetName(),
			errDeleteInUse)
	}

	return nil, nil
}

func validateCertificate(cert *enginefeatureseverestv1alpha1.SplitHorizonDNSConfigTLSCertificateSpec) field.ErrorList {
	var allErrs field.ErrorList
	if cert == nil {
		allErrs = append(allErrs, errRequiredField(certificatePath))
		return allErrs
	}

	if cert.CaCertFile == "" {
		// return errTLSCaCertEmpty
		allErrs = append(allErrs, errRequiredField(caCertFilePath))
	} else if !utils.IsBase64Encoded(cert.CaCertFile) {
		allErrs = append(allErrs, errCertWrongEncodingField(caCertFilePath, cert.CaCertFile))
		// return errTLSCaCertWrongEncoding
	}

	if cert.CertFile == "" {
		// return errTLSCertEmpty
		allErrs = append(allErrs, errRequiredField(certFilePath))
	} else if !utils.IsBase64Encoded(cert.CertFile) {
		allErrs = append(allErrs, errCertWrongEncodingField(certFilePath, cert.CertFile))
		// return errTLSCertWrongEncoding
	}

	if cert.KeyFile == "" {
		// return errTLSKeyEmpty
		allErrs = append(allErrs, errRequiredField(keyFilePath))
	} else if !utils.IsBase64Encoded(cert.KeyFile) {
		// return errTLSKeyWrongEncoding
		allErrs = append(allErrs, errCertWrongEncodingField(keyFilePath, cert.KeyFile))
	}

	if len(allErrs) == 0 {
		return nil
	}

	return allErrs
}

func validateSecret(ctx context.Context, c client.Client, namespace, name string) field.ErrorList {
	var allErrs field.ErrorList
	secret := corev1.Secret{}
	err := c.Get(ctx, types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}, &secret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			allErrs = append(allErrs, field.NotFound(secretNamePath, name))
		} else {
			allErrs = append(allErrs, field.InternalError(secretNamePath, err))
		}
		return allErrs
	}
	// Secret found. Check that it contains required fields.
	if secret.Type != corev1.SecretTypeTLS {
		allErrs = append(allErrs, field.Invalid(secretNamePath, name, fmt.Sprintf("the secret must be of type '%s'", corev1.SecretTypeTLS)))
	}
	if _, ok := secret.Data["ca.crt"]; !ok {
		allErrs = append(allErrs, field.Required(secretNamePath, "ca.crt field is missed in the secret"))
	}
	if _, ok := secret.Data["tls.crt"]; !ok {
		allErrs = append(allErrs, field.Required(secretNamePath, "tls.crt field is missed in the secret"))
	}
	if _, ok := secret.Data["tls.key"]; !ok {
		allErrs = append(allErrs, field.Required(secretNamePath, "tls.key field is missed in the secret"))
	}

	if len(allErrs) == 0 {
		return nil
	}

	return allErrs
}
