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
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// BackupStorageTypeS3 is a type of S3 object storage.
	BackupStorageTypeS3 BackupStorageType = "s3"
	// BackupStorageTypeAzure is a type of azure blob storage.
	BackupStorageTypeAzure BackupStorageType = "azure"
	// BackupStorageTypeLocal is a type of local storage.
	BackupStorageTypeLocal BackupStorageType = "local"
)

// BackupStorageType is a type of backup storage.
type BackupStorageType string

const (
	// PGInitLocalBackupStorageNameTmpl is a template for the name of the local backupstorage
	// created for bootstrapping PG clusters.
	PGInitLocalBackupStorageNameTmpl = "%s-pg-init"
)

const (
	// PGInitLocalBackupStorageReferenceLabel is a label set on a backup storage to indicate
	// that it was created for bootstrapping PG a cluster.
	// The value of this label is the name of the PG cluster that owns this backup storage.
	PGInitLocalBackupStorageReferenceLabel = "everest.percona.com/pg-init"
)

// LocalBackupStorageName returns the name of the local backupstorage.
func LocalBackupStorageName(clusterName string) string {
	return fmt.Sprintf(PGInitLocalBackupStorageNameTmpl, clusterName)
}

// BackupStorageSpec defines the desired state of BackupStorage.
type BackupStorageSpec struct {
	// Type is a type of backup storage.
	// +kubebuilder:validation:Enum=s3;azure;local
	Type BackupStorageType `json:"type"`
	// PVCSpec is a spec of PVC for local storage.
	// Ignored if Type is not local.
	PVCSpec *corev1.PersistentVolumeClaimSpec `json:"pvcSpec,omitempty"`
	// Bucket is a name of bucket.
	Bucket string `json:"bucket"`
	// Region is a region where the bucket is located.
	Region string `json:"region,omitempty"`
	// EndpointURL is an endpoint URL of backup storage.
	EndpointURL string `json:"endpointURL,omitempty"`
	// VerifyTLS is set to ensure TLS/SSL verification.
	// If unspecified, the default value is true.
	//
	// +kubebuilder:default:=true
	VerifyTLS *bool `json:"verifyTLS,omitempty"`
	// ForcePathStyle is set to use path-style URLs.
	// If unspecified, the default value is false.
	//
	// +kubebuilder:default:=false
	ForcePathStyle *bool `json:"forcePathStyle,omitempty"`

	// Description stores description of a backup storage.
	Description string `json:"description,omitempty"`
	// CredentialsSecretName is the name of the secret with credentials.
	CredentialsSecretName string `json:"credentialsSecretName"`
	// AllowedNamespaces is the list of namespaces where the operator will copy secrets provided in the CredentialsSecretsName.
	AllowedNamespaces []string `json:"allowedNamespaces,omitempty"`
}

// BackupStorageStatus defines the observed state of BackupStorage.
type BackupStorageStatus struct {
	UsedNamespaces map[string]bool `json:"usedNamespaces"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// BackupStorage is the Schema for the backupstorages API.
type BackupStorage struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BackupStorageSpec   `json:"spec,omitempty"`
	Status BackupStorageStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// BackupStorageList contains a list of BackupStorage.
type BackupStorageList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BackupStorage `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BackupStorage{}, &BackupStorageList{})
}

// UpdateNamespacesList updates the list of namespaces that use the backupStorage.
func (b *BackupStorage) UpdateNamespacesList(namespace string) bool {
	if b.Status.UsedNamespaces == nil {
		b.Status.UsedNamespaces = make(map[string]bool)
	}
	if _, ok := b.Status.UsedNamespaces[namespace]; ok {
		return false
	}
	b.Status.UsedNamespaces[namespace] = true
	return true
}

// DeleteUsedNamespace deletes the namespace from the usedNamespaces list.
func (b *BackupStorage) DeleteUsedNamespace(namespace string) bool {
	if b.Status.UsedNamespaces == nil {
		return false
	}
	if _, ok := b.Status.UsedNamespaces[namespace]; ok {
		delete(b.Status.UsedNamespaces, namespace)
		return true
	}
	return false
}

// IsNamespaceAllowed checks the namespace against allowedNamespaces and returns if it's allowed to use.
func (b *BackupStorage) IsNamespaceAllowed(namespace string) bool {
	if len(b.Spec.AllowedNamespaces) == 0 {
		return true
	}
	for _, ns := range b.Spec.AllowedNamespaces {
		ns := ns
		if ns == namespace {
			return true
		}
	}
	return false
}
