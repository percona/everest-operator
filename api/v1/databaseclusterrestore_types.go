// dbaas-operator
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

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// BackupStorageFilesystem represents file system storage type.
	BackupStorageFilesystem BackupStorageType = "filesystem"
	// BackupStorageS3 represents s3 storage.
	BackupStorageS3 BackupStorageType = "s3"
	// BackupStorageGCS represents Google Cloud storage.
	BackupStorageGCS BackupStorageType = "gcs"
	// BackupStorageAzure represents azure storage.
	BackupStorageAzure BackupStorageType = "azure"
)

type (
	// RestoreState represents state of restoration.
	RestoreState string
	// BackupStorageType represents backup storage type.
	BackupStorageType string

	// DatabaseClusterRestoreSpec defines the desired state of DatabaseClusterRestore.
	DatabaseClusterRestoreSpec struct {
		DatabaseCluster string        `json:"databaseCluster"`
		DatabaseType    EngineType    `json:"databaseType"`
		BackupName      string        `json:"backupName,omitempty"`
		BackupSource    *BackupSource `json:"backupSource,omitempty"`
	}
	// BackupSource represents settings of a source where to get a backup to run restoration.
	BackupSource struct {
		Destination           string                     `json:"destination,omitempty"`
		StorageName           string                     `json:"storageName,omitempty"`
		S3                    *BackupStorageProviderSpec `json:"s3,omitempty"`
		Azure                 *BackupStorageProviderSpec `json:"azure,omitempty"`
		StorageType           BackupStorageType          `json:"storage_type"`
		Image                 string                     `json:"image,omitempty"`
		SSLSecretName         string                     `json:"sslSecretName,omitempty"`
		SSLInternalSecretName string                     `json:"sslInternalSecretName,omitempty"`
		VaultSecretName       string                     `json:"vaultSecretName,omitempty"`
	}
)

// DatabaseClusterRestoreStatus defines the observed state of DatabaseClusterRestore.
type DatabaseClusterRestoreStatus struct {
	State         RestoreState       `json:"state,omitempty"`
	CompletedAt   *metav1.Time       `json:"completed,omitempty"`
	LastScheduled *metav1.Time       `json:"lastscheduled,omitempty"`
	Conditions    []metav1.Condition `json:"conditions,omitempty"`
	Message       string             `json:"message,omitempty"`
	Destination   string             `json:"destination,omitempty"`
	StorageName   string             `json:"storageName,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName="dbc-restore";"dbc-restores"
// +kubebuilder:printcolumn:name="Cluster",type="string",JSONPath=".spec.databaseCluster",description="Cluster name"
// +kubebuilder:printcolumn:name="Storage",type="string",JSONPath=".status.storageName",description="Storage name from pxc spec"
// +kubebuilder:printcolumn:name="Destination",type="string",JSONPath=".status.destination",description="Backup destination"
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.state",description="Job status"
// +kubebuilder:printcolumn:name="Completed",type="date",JSONPath=".status.completed",description="Completed time"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// DatabaseClusterRestore is the Schema for the databaseclusterrestores API.
type DatabaseClusterRestore struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DatabaseClusterRestoreSpec   `json:"spec,omitempty"`
	Status DatabaseClusterRestoreStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// DatabaseClusterRestoreList contains a list of DatabaseClusterRestore.
type DatabaseClusterRestoreList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DatabaseClusterRestore `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DatabaseClusterRestore{}, &DatabaseClusterRestoreList{})
}
