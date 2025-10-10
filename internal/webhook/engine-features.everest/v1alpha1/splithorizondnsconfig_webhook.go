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

package v1alpha1

import (
	"context"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	enginefeatureseverestv1alpha1 "github.com/percona/everest-operator/api/engine-features.everest/v1alpha1"
	"github.com/percona/everest-operator/utils"
)

// errors for SplitHorizonDNSConfig webhook validation
var (
	// Base domain name suffix errors
	errInvalidBaseDomainNameSuffix = func(err error) error {
		return fmt.Errorf(".spec.baseDomainNameSuffix is not a valid DNS subdomain: %v", err)
	}
	errTlsSecretNameEmpty     = errors.New(".spec.tls.secretName can not be empty")
	errTlsCertificateEmpty    = errors.New(".spec.tls.certificate can not be empty")
	errTlsCaCertEmpty         = errors.New(".spec.tls.certificate.caCertFile can not be empty")
	errTlsCaCertWrongEncoding = errors.New(".spec.tls.certificate.caCertFile is not base64-encoded")
	errTlsCertEmpty           = errors.New(".spec.tls.certificate.certFile can not be empty")
	errTlsCertWrongEncoding   = errors.New(".spec.tls.certificate.certFile is not base64-encoded")
	errTlsKeyEmpty            = errors.New(".spec.tls.certificate.keyFile can not be empty")
	errTlsKeyWrongEncoding    = errors.New(".spec.tls.certificate.keyFile is not base64-encoded")
	errDeleteInUse            = func(name string) error {
		return fmt.Errorf("split-horizon dns config with name='%s' is used by some DB cluster and cannot be deleted", name)
	}
	errBaseDomainNameSuffixImmutable = errors.New(".spec.baseDomainNameSuffix field is immutable and cannot be changed")
	errSecretNameImmutable           = errors.New(".spec.tls.secretName field is immutable and cannot be changed")
)

// nolint:unused
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
// +kubebuilder:webhook:path=/validate-engine-features-everest-percona-com-v1alpha1-splithorizondnsconfig,mutating=false,failurePolicy=fail,sideEffects=None,groups=engine-features.everest.percona.com,resources=splithorizondnsconfigs,verbs=create;update;delete,versions=v1alpha1,name=vsplithorizondnsconfig-v1alpha1.kb.io,admissionReviewVersions=v1

// SplitHorizonDNSConfigCustomValidator struct is responsible for validating the SplitHorizonDNSConfig resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type SplitHorizonDNSConfigCustomValidator struct {
	Client client.Client
}

var _ webhook.CustomValidator = &SplitHorizonDNSConfigCustomValidator{}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type SplitHorizonDNSConfig.
func (v *SplitHorizonDNSConfigCustomValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	shdc, ok := obj.(*enginefeatureseverestv1alpha1.SplitHorizonDNSConfig)
	if !ok {
		return nil, fmt.Errorf("expected a SplitHorizonDNSConfig object but got %T", obj)
	}

	logger := logf.FromContext(ctx).WithName("SplitHorizonDNSConfigValidator").WithValues(
		"name", shdc.GetName(),
		"namespace", shdc.GetNamespace(),
	)

	logger.Info("Validation for SplitHorizonDNSConfig upon creation")

	if err := utils.ValidateRFC1035(shdc.GetName(), "metadata.name"); err != nil {
		return nil, err
	}

	if err := utils.ValidateDNSName(shdc.Spec.BaseDomainNameSuffix); err != nil {
		return nil, errInvalidBaseDomainNameSuffix(err)
	}

	secretName := shdc.Spec.TLS.SecretName
	if secretName == "" {
		return nil, errTlsSecretNameEmpty
	}

	if shdc.Spec.TLS.Certificate == nil {
		// .spec.tls.certificate is not provided, so .spec.tls.secretName must be used.
		// Check that provided secret exists.
		secret := corev1.Secret{}
		if err := v.Client.Get(ctx, types.NamespacedName{
			Name:      secretName,
			Namespace: shdc.GetNamespace(),
		}, &secret); err != nil {
			return nil, fmt.Errorf("failed to get secrets %s: %w", secretName, err)
		}
	} else {
		// Both Certificate and SecretName are provided.
		// Certificate fields must be non-empty.
		// Secret is not checked, as Certificate is provided.
		if err := validateCertificate(shdc.Spec.TLS.Certificate); err != nil {
			return nil, err
		}
	}
	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type SplitHorizonDNSConfig.
func (v *SplitHorizonDNSConfigCustomValidator) ValidateUpdate(ctx context.Context, oldObj, newObj runtime.Object) (admission.Warnings, error) {
	oldShdc, ok := oldObj.(*enginefeatureseverestv1alpha1.SplitHorizonDNSConfig)
	if !ok {
		return nil, fmt.Errorf("expected a SplitHorizonDNSConfig object for the newObj but got %T", newObj)
	}

	newShdc, ok := newObj.(*enginefeatureseverestv1alpha1.SplitHorizonDNSConfig)
	if !ok {
		return nil, fmt.Errorf("expected a SplitHorizonDNSConfig object for the newObj but got %T", newObj)
	}

	logger := logf.FromContext(ctx).WithName("SplitHorizonDNSConfigValidator").WithValues(
		"name", newShdc.GetName(),
		"namespace", newShdc.GetNamespace(),
	)

	logger.Info("Validation for SplitHorizonDNSConfig upon update")

	if !newShdc.DeletionTimestamp.IsZero() {
		return nil, nil
	}

	// TODO: PSMDB supports this update. Need to handle it later.
	if newShdc.Spec.BaseDomainNameSuffix != oldShdc.Spec.BaseDomainNameSuffix {
		return nil, errBaseDomainNameSuffixImmutable
	}

	if newShdc.Spec.TLS.SecretName != oldShdc.Spec.TLS.SecretName {
		return nil, errSecretNameImmutable
	}

	// Only .spec.tls.certificate can be updated.
	// If it is provided, its fields must be non-empty and properly encoded (base64).
	if err := validateCertificate(newShdc.Spec.TLS.Certificate); err != nil {
		return nil, err
	}

	return nil, nil
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
		return nil, errDeleteInUse(shdc.GetName())
	}

	return nil, nil
}

func validateCertificate(cert *enginefeatureseverestv1alpha1.SplitHorizonDNSConfigTLSCertificateSpec) error {
	if cert == nil {
		return errTlsCertificateEmpty
	}

	if cert.CaCertFile == "" {
		return errTlsCaCertEmpty
	} else if !utils.IsBase64Encoded(cert.CaCertFile) {
		return errTlsCaCertWrongEncoding
	}

	if cert.CertFile == "" {
		return errTlsCertEmpty
	} else if !utils.IsBase64Encoded(cert.CertFile) {
		return errTlsCertWrongEncoding
	}

	if cert.KeyFile == "" {
		return errTlsKeyEmpty
	} else if !utils.IsBase64Encoded(cert.KeyFile) {
		return errTlsKeyWrongEncoding
	}

	return nil
}
