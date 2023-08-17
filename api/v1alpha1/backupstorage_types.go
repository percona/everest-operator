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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// BackupStorageTypeS3 is a type of S3 object storage.
	BackupStorageTypeS3 BackupStorageType = "s3"
)

// BackupStorageType is a type of backup storage.
type BackupStorageType string

// BackupStorageSpec defines the desired state of BackupStorage.
type BackupStorageSpec struct {
	// Type is a type of backup storage. Currently only S3 is supported.
	// +kubebuilder:validation:Enum=s3
	Type BackupStorageType `json:"type"`
	// Bucket is a name of bucket.
	Bucket string `json:"bucket"`
	// Region is a region where the bucket is located.
	Region string `json:"region"`
	// EndpointURL is an endpoint URL of backup storage.
	EndpointURL string `json:"endpointURL"`
	// CredentialsSecretName is the name of the secret with credentials.
	CredentialsSecretName string `json:"credentialsSecretName"`
}

// BackupStorageStatus defines the observed state of BackupStorage.
type BackupStorageStatus struct{}

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
