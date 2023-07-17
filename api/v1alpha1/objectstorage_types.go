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
	// ObjectStorageTypeS3 is a type of S3 object storage.
	ObjectStorageTypeS3 ObjectStorageType = "s3"
)

// ObjectStorageType is a type of object storage.
type ObjectStorageType string

// ObjectStorageSpec defines the desired state of ObjectStorage.
type ObjectStorageSpec struct {
	// Type is a type of object storage. Currently only S3 is supported.
	// +kubebuilder:validation:Enum=s3
	Type ObjectStorageType `json:"type"`
	// Bucket is a name of bucket.
	Bucket string `json:"bucket"`
	// Region is a region where the bucket is located.
	Region string `json:"region"`
	// EndpointURL is an endpoint URL of object storage.
	EndpointURL string `json:"endpointURL"`
	// CredentialsSecretName is the name of the secret with credentials.
	CredentialsSecretName string `json:"credentialsSecretName"`
}

// ObjectStorageStatus defines the observed state of ObjectStorage.
type ObjectStorageStatus struct{}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// ObjectStorage is the Schema for the objectstorages API.
type ObjectStorage struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ObjectStorageSpec   `json:"spec,omitempty"`
	Status ObjectStorageStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ObjectStorageList contains a list of ObjectStorage.
type ObjectStorageList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ObjectStorage `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ObjectStorage{}, &ObjectStorageList{})
}
