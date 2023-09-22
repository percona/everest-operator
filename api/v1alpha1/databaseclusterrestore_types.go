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

// RestoreState represents state of restoration.
type RestoreState string

// DatabaseClusterRestoreSpec defines the desired state of DatabaseClusterRestore.
type DatabaseClusterRestoreSpec struct {
	// DBClusterName defines the cluster name to restore.
	DBClusterName string `json:"dbClusterName"`
	// DataSource defines a data source for restoration.
	DataSource DataSource `json:"dataSource"`
}

// DatabaseClusterRestoreStatus defines the observed state of DatabaseClusterRestore.
type DatabaseClusterRestoreStatus struct {
	State       RestoreState `json:"state,omitempty"`
	CompletedAt *metav1.Time `json:"completed,omitempty"`
	Message     string       `json:"message,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:subresource:status
// +kubebuilder:resource:shortName=dbrestore;dbr
// +kubebuilder:printcolumn:name="Cluster",type="string",JSONPath=".spec.dbClusterName",description="Cluster name"
// +kubebuilder:printcolumn:name="Backup",type="string",JSONPath=".spec.dataSource.dbClusterBackupName",description="DBClusterBackup name"
// +kubebuilder:printcolumn:name="Path",type="string",JSONPath=".spec.dataSource.backupSource.path",description="Backup path"
// +kubebuilder:printcolumn:name="Storage",type="string",JSONPath=".spec.dataSource.backupSource.backupStorageName",description="Storage name"
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
