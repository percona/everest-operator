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

package psmdb

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"time"

	"github.com/AlekSi/pointer"
	psmdbv1 "github.com/percona/percona-server-mongodb-operator/pkg/apis/psmdb/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	enginefeatureseverestv1alpha1 "github.com/percona/everest-operator/api/enginefeatures.everest/v1alpha1"
)

var (
	errShardingNotSupported = errors.New("sharding is not supported for SplitHorizon DNS feature")
	validityNotAfter        = time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC)
)

// EngineFeaturesApplier is responsible for applying engine features to the PerconaServerMongoDB spec.
type EngineFeaturesApplier struct {
	*Provider
}

// NewEngineFeaturesApplier creates a new engineFeaturesApplier.
func NewEngineFeaturesApplier(p *Provider) *EngineFeaturesApplier {
	return &EngineFeaturesApplier{
		Provider: p,
	}
}

// ApplyFeatures applies the engine features to the PerconaServerMongoDB spec.
func (a *EngineFeaturesApplier) ApplyFeatures(ctx context.Context) error {
	if pointer.Get(a.DB.Spec.EngineFeatures).PSMDB == nil {
		// Nothing to do
		return nil
	}

	var err error
	if err = a.applySplitHorizonDNSConfig(ctx); err != nil {
		return err
	}
	return nil
}

// applySplitHorizonDNSConfig applies the SplitHorizon DNS configuration to the PerconaServerMongoDB spec.
func (a *EngineFeaturesApplier) applySplitHorizonDNSConfig(ctx context.Context) error {
	database := a.DB
	psmdb := a.PerconaServerMongoDB
	shdcName := database.Spec.EngineFeatures.PSMDB.SplitHorizonDNSConfigName
	if shdcName == "" {
		// SplitHorizon DNS feature is not enabled
		return nil
	}

	if pointer.Get(database.Spec.Sharding).Enabled {
		// For the time being SplitHorizon DNS feature can be applied for unsharded PSMDB clusters only.
		// Later we may consider adding support for sharded clusters as well.
		return errShardingNotSupported
	}

	shdc := &enginefeatureseverestv1alpha1.SplitHorizonDNSConfig{}
	if err := a.C.Get(ctx, types.NamespacedName{Namespace: a.DB.GetNamespace(), Name: shdcName}, shdc); err != nil {
		return err
	}

	shdcCaSecret := &corev1.Secret{}
	if err := a.C.Get(ctx, types.NamespacedName{Namespace: shdc.GetNamespace(), Name: shdc.Spec.TLS.SecretName}, shdcCaSecret); err != nil {
		return err
	}

	horSpec := psmdbv1.HorizonsSpec{}
	for i := range int(database.Spec.Engine.Replicas) {
		horSpec[fmt.Sprintf("%s-rs0-%d", database.GetName(), i)] = map[string]string{
			"external": fmt.Sprintf("%s-rs0-%d-%s.%s",
				database.GetName(),
				i,
				database.GetNamespace(),
				shdc.Spec.BaseDomainNameSuffix),
		}
	}
	psmdb.Spec.Replsets[0].Horizons = horSpec

	needCreateSecret := false // do not generate server certificate on each reconciliation loop
	psmdbSplitHorizonSecretName := fmt.Sprintf("%s-sh-cert", database.GetName())

	psmdbSplitHorizonSecret := &corev1.Secret{}
	if err := a.C.Get(ctx, types.NamespacedName{Namespace: database.GetNamespace(), Name: psmdbSplitHorizonSecretName}, psmdbSplitHorizonSecret); err != nil {
		if k8serrors.IsNotFound(err) {
			needCreateSecret = true
		} else {
			return err
		}
	}

	if needCreateSecret {
		// Generate server TLS certificate for SplitHorizon DNS domains
		psmdbSplitHorizonDomains := []string{
			// Internal SANs
			"localhost",
			fmt.Sprintf("%s-rs0", database.GetName()),
			fmt.Sprintf("*.%s-rs0", database.GetName()),
			fmt.Sprintf("%s-rs0.%s", database.GetName(), database.GetNamespace()),
			fmt.Sprintf("*.%s-rs0.%s", database.GetName(), database.GetNamespace()),
			fmt.Sprintf("%s-rs0.%s.svc.cluster.local", database.GetName(), database.GetNamespace()),
			fmt.Sprintf("*.%s-rs0.%s.svc.cluster.local", database.GetName(), database.GetNamespace()),
			// External SANs
			fmt.Sprintf("*.%s", shdc.Spec.BaseDomainNameSuffix),
		}
		psmdbSplitHorizonServerCertBytes, psmdbSplitHorizonServerPrivKeyBytes, err := issueSplitHorizonCertificate(shdcCaSecret.Data["ca.crt"], shdcCaSecret.Data["ca.key"], psmdbSplitHorizonDomains)
		if err != nil {
			return fmt.Errorf("issue split-horizon server TLS certificate: %w", err)
		}

		// Store generated server TLS certificate in a secret. It is DB specific.
		psmdbSplitHorizonSecret.SetName(psmdbSplitHorizonSecretName)
		psmdbSplitHorizonSecret.SetNamespace(database.GetNamespace())
		if _, err = controllerutil.CreateOrUpdate(ctx, a.C, psmdbSplitHorizonSecret, func() error {
			psmdbSplitHorizonSecret.Data = map[string][]byte{
				"tls.crt": []byte(psmdbSplitHorizonServerCertBytes),
				"tls.key": []byte(psmdbSplitHorizonServerPrivKeyBytes),
				"ca.crt":  shdcCaSecret.Data["ca.crt"],
			}
			psmdbSplitHorizonSecret.Type = corev1.SecretTypeTLS

			if err = controllerutil.SetControllerReference(database, psmdbSplitHorizonSecret, a.C.Scheme()); err != nil {
				return err
			}

			return nil
		}); err != nil {
			return fmt.Errorf("failed to create server TLS certificate secret: %w", err)
		}
	}

	// set reference to secret with certificate for external domain
	if psmdb.Spec.Secrets == nil {
		psmdb.Spec.Secrets = &psmdbv1.SecretsSpec{}
	}
	psmdb.Spec.Secrets.SSL = psmdbSplitHorizonSecret.GetName()

	return nil
}

// issueSplitHorizonCertificate generates server TLS certificate signed by the provided CA certificate and private key,
// with SANs for the provided hosts.
// It returns the generated server TLS certificate, private key in PEM format and error.
func issueSplitHorizonCertificate(caCert, caPrivKey []byte, hosts []string) (string, string, error) {
	caDecoded, _ := pem.Decode(caCert)
	ca, err := x509.ParseCertificate(caDecoded.Bytes)
	if err != nil {
		return "", "", fmt.Errorf("parse CA certificate: %w", err)
	}

	caPrivKeyDecoded, _ := pem.Decode(caPrivKey)
	caKey, err := x509.ParsePKCS1PrivateKey(caPrivKeyDecoded.Bytes)

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	serialNumber, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return "", "", fmt.Errorf("generate serial number for client: %w", err)
	}

	serverCertTemplate := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"PSMDB"},
		},
		NotBefore:             time.Now(),
		NotAfter:              validityNotAfter,
		IPAddresses:           []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},
		DNSNames:              hosts,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  false,
	}
	// Create server certificate private key
	certPrivKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", fmt.Errorf("generate client key: %w", err)
	}

	// Create and sign server certificate with CA
	serverCertBytes, err := x509.CreateCertificate(rand.Reader, &serverCertTemplate, ca, &certPrivKey.PublicKey, caKey)
	if err != nil {
		return "", "", fmt.Errorf("generate server certificate: %w", err)
	}
	serverCertPem := &bytes.Buffer{}
	err = pem.Encode(serverCertPem, &pem.Block{Type: "CERTIFICATE", Bytes: serverCertBytes})
	if err != nil {
		return "", "", fmt.Errorf("encode server certificate: %w", err)
	}

	serverKeyPem := &bytes.Buffer{}
	block := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(certPrivKey)}
	err = pem.Encode(serverKeyPem, block)
	if err != nil {
		return "", "", fmt.Errorf("encode RSA private key: %w", err)
	}

	return serverCertPem.String(), serverKeyPem.String(), nil
}
