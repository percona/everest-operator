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
	"encoding/json"
	"time"

	pgv2 "github.com/percona/percona-postgresql-operator/pkg/apis/pgv2.percona.com/v2"
	psmdbv1 "github.com/percona/percona-server-mongodb-operator/pkg/apis/psmdb/v1"
	pxcv1 "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RestoreState represents state of restoration.
type RestoreState string

// PITRType represents type of Point-in-time recovery.
type PITRType string

// DateFormat is the date format used in the user input.
const (
	DateFormat      = "2006-01-02T15:04:05Z"
	DateFormatSpace = "2006-01-02 15:04:05"
)

const (
	// PITRTypeDate is Point-in-time recovery type based on the specific date.
	PITRTypeDate PITRType = "date"
	// PITRTypeLatest is Point-in-time recovery type based on the latest date.
	PITRTypeLatest PITRType = "latest"
)

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

// PITR represents a specification to configure point in time recovery for a database backup/restore.
type PITR struct {
	// Type is the type of recovery.
	// +kubebuilder:validation:Enum:=date;latest
	// +kubebuilder:default:=date
	Type PITRType `json:"type,omitempty"`
	// Date is the UTC date to recover to. The accepted format: "2006-01-02T15:04:05Z".
	Date *RestoreDate `json:"date,omitempty"`
}

// RestoreDate is a data type for better time.Time support, the same approach as used in psmdb.
// +kubebuilder:validation:Type=string
type RestoreDate struct {
	metav1.Time `json:",inline"`
}

// OpenAPISchemaType returns a schema type for OperAPI specification.
func (RestoreDate) OpenAPISchemaType() []string { return []string{"string"} }

// OpenAPISchemaFormat returns a format for OperAPI specification.
func (RestoreDate) OpenAPISchemaFormat() string { return "" }

// MarshalJSON marshals JSON.
func (t RestoreDate) MarshalJSON() ([]byte, error) {
	if t.IsZero() {
		return []byte("null"), nil
	}

	timeStr := t.Time.Format(DateFormat)
	return json.Marshal(timeStr)
}

// UnmarshalJSON unmarshals JSON.
func (t *RestoreDate) UnmarshalJSON(b []byte) error {
	if len(b) == 4 && string(b) == "null" {
		t.Time = metav1.NewTime(time.Time{})
		return nil
	}

	var str string

	if err := json.Unmarshal(b, &str); err != nil {
		return err
	}

	pt, err := time.Parse(DateFormat, str)
	if err != nil {
		return err
	}

	t.Time = metav1.NewTime(pt)

	return nil
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

// IsComplete indicates if the restoration process is complete (regardless successful or not).
func (r *DatabaseClusterRestore) IsComplete(engineType EngineType) bool {
	switch engineType {
	case DatabaseEnginePXC:
		return isPXCRestoreStatusComplete(pxcv1.BcpRestoreStates(r.Status.State))
	case DatabaseEnginePSMDB:
		return isPSMDBRestoreStatusComplete(psmdbv1.RestoreState(r.Status.State))
	case DatabaseEnginePostgresql:
		return isPGRestoreStatusComplete(pgv2.PGRestoreState(r.Status.State))
	}
	return true
}

func isPXCRestoreStatusComplete(status pxcv1.BcpRestoreStates) bool {
	switch status {
	case pxcv1.RestoreNew:
		return false
	case pxcv1.RestoreStarting:
		return false
	case pxcv1.RestoreStopCluster:
		return false
	case pxcv1.RestoreRestore:
		return false
	case pxcv1.RestoreStartCluster:
		return false
	case pxcv1.RestorePITR:
		return false
	case pxcv1.RestoreFailed:
		return true
	case pxcv1.RestoreSucceeded:
		return true
	}
	return true
}

func isPSMDBRestoreStatusComplete(status psmdbv1.RestoreState) bool {
	switch status {
	case psmdbv1.RestoreStateNew:
		return false
	case psmdbv1.RestoreStateWaiting:
		return false
	case psmdbv1.RestoreStateRequested:
		return false
	case psmdbv1.RestoreStateRunning:
		return false
	case psmdbv1.RestoreStateRejected:
		return true
	case psmdbv1.RestoreStateError:
		return true
	case psmdbv1.RestoreStateReady:
		return true
	}
	return true
}

func isPGRestoreStatusComplete(status pgv2.PGRestoreState) bool {
	switch status {
	case pgv2.RestoreNew:
		return false
	case pgv2.RestoreStarting:
		return false
	case pgv2.RestoreRunning:
		return false
	case pgv2.RestoreFailed:
		return true
	case pgv2.RestoreSucceeded:
		return true
	}
	return true
}
