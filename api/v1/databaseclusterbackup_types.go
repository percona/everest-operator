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

// BackupState is used to represent cluster's state.
type BackupState string

// DatabaseClusterBackupSpec defines the desired state of DatabaseClusterBackup.
type DatabaseClusterBackupSpec struct {
	// Name is the backup name.
	Name string `json:"name"`
	// DBClusterName is the original database cluster name.
	DBClusterName string `json:"dbClusterName"`
	// Destination is the full path to the backup.
	Destination string `json:"destination"`
	// BackupSource is the object with the storage location info.
	BackupSource BackupSource `json:"backupSource"`
}

// DatabaseClusterBackupStatus defines the observed state of DatabaseClusterBackup.
type DatabaseClusterBackupStatus struct {
	// Completed is the time when the job was completed.
	Completed *metav1.Time `json:"completed"`
	// State is the DatabaseBackup state.
	State BackupState `json:"state"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
// +kubebuilder:resource:shortName=dbbackup;dbb
// +kubebuilder:printcolumn:name="Cluster",type="string",JSONPath=".spec.dbClusterName",description="The original database cluster name"
// +kubebuilder:printcolumn:name="Destination",type="string",JSONPath=".spec.destination",description="Backup destination"
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.state",description="Job status"
// +kubebuilder:printcolumn:name="Completed",type="date",JSONPath=".status.completed",description="Time the job was completed"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description="Age of the resource"

// DatabaseClusterBackup is the Schema for the databaseclusterbackups API.
type DatabaseClusterBackup struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DatabaseClusterBackupSpec   `json:"spec,omitempty"`
	Status DatabaseClusterBackupStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// DatabaseClusterBackupList contains a list of DatabaseClusterBackup.
type DatabaseClusterBackupList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DatabaseClusterBackup `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DatabaseClusterBackup{}, &DatabaseClusterBackupList{})
}
