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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// AppStateUnknown is an unknown state.
	AppStateUnknown AppState = "unknown"
	// AppStateInit is a initializing state.
	AppStateInit AppState = "initializing"
	// AppStatePaused is a paused state.
	AppStatePaused AppState = "paused"
	// AppStatePausing is a pausing state.
	AppStatePausing AppState = "pausing"
	// AppStateStopping is a stopping state.
	AppStateStopping AppState = "stopping"
	// AppStateReady is a ready state.
	AppStateReady AppState = "ready"
	// AppStateError is an error state.
	AppStateError AppState = "error"
	// AppStateRestoring is a restoring state.
	AppStateRestoring AppState = "restoring"

	// ExposeTypeInternal is an internal expose type.
	ExposeTypeInternal ExposeType = "internal"
	// ExposeTypeExternal is an external expose type.
	ExposeTypeExternal ExposeType = "external"

	// ProxyTypeMongos is a mongos proxy type.
	ProxyTypeMongos ProxyType = "mongos"
	// ProxyTypeHAProxy is a HAProxy proxy type.
	ProxyTypeHAProxy ProxyType = "haproxy"
	// ProxyTypeProxySQL is a ProxySQL proxy type.
	ProxyTypeProxySQL ProxyType = "proxysql"
	// ProxyTypePGBouncer is a PGBouncer proxy type.
	ProxyTypePGBouncer ProxyType = "pgbouncer"
)

type (
	// LoadBalancerType contains supported loadbalancers. It can be proxysql or haproxy
	// for PXC clusters, mongos for PSMDB clusters or pgbouncer for Postgresql clusters.
	LoadBalancerType string
	// AppState is used to represent cluster's state.
	AppState string
	// BackupStorageProviderSpec represents set of settings to configure cloud provider.
	BackupStorageProviderSpec struct {
		// A container name is a valid DNS name that conforms to the Azure naming rules.
		ContainerName string `json:"containerName,omitempty"`

		Bucket            string `json:"bucket,omitempty"`
		Prefix            string `json:"prefix,omitempty"`
		CredentialsSecret string `json:"credentialsSecret"`
		Region            string `json:"region,omitempty"`
		EndpointURL       string `json:"endpointUrl,omitempty"`

		// STANDARD, NEARLINE, COLDLINE, ARCHIVE for GCP
		// Hot (Frequently accessed or modified data), Cool (Infrequently accessed or modified data), Archive (Rarely accessed or modified data) for Azure.
		StorageClass string `json:"storageClass,omitempty"`
	}
)

// Storage is the storage configuration.
type Storage struct {
	// Size is the size of the persistent volume claim
	Size resource.Quantity `json:"size"`
	// Class is the storage class to use for the persistent volume claim
	Class *string `json:"class,omitempty"`
}

// Resources are the resource requirements.
type Resources struct {
	// CPU is the CPU resource requirements
	CPU resource.Quantity `json:"cpu,omitempty"`
	// Memory is the memory resource requirements
	Memory resource.Quantity `json:"memory,omitempty"`
}

// Engine is the engine configuration.
type Engine struct {
	// Type is the engine type
	Type EngineType `json:"type"`
	// Version is the engine version
	Version string `json:"version,omitempty"`
	// Replicas is the number of engine replicas
	Replicas int32 `json:"replicas,omitempty"`
	// Storage is the engine storage configuration
	Storage Storage `json:"storage"`
	// Resources are the resource limits for each engine replica.
	// If not set, resource limits are not imposed
	Resources Resources `json:"resources,omitempty"`
	// Config is the engine configuration
	Config string `json:"config,omitempty"`
}

// ExposeType is the expose type.
type ExposeType string

// Expose is the expose configuration.
type Expose struct {
	// Type is the expose type, can be internal or external
	// +kubebuilder:validation:Enum:=internal;external
	// +kubebuilder:default:=internal
	Type ExposeType `json:"type,omitempty"`
	// IPSourceRanges is the list of IP source ranges (CIDR notation)
	// to allow access from. If not set, there is no limitations
	IPSourceRanges []string `json:"ipSourceRanges,omitempty"`
}

// ProxyType is the proxy type.
type ProxyType string

// Proxy is the proxy configuration.
type Proxy struct {
	// Type is the proxy type
	// +kubebuilder:validation:Enum:=mongos;haproxy;proxysql;pgbouncer
	Type ProxyType `json:"type,omitempty"`
	// Replicas is the number of proxy replicas
	Replicas *int32 `json:"replicas,omitempty"`
	// Config is the proxy configuration
	Config string `json:"config,omitempty"`
	// Expose is the proxy expose configuration
	Expose Expose `json:"expose,omitempty"`
	// Resources are the resource limits for each proxy replica.
	// If not set, resource limits are not imposed
	Resources Resources `json:"resources,omitempty"`
}

// DataSource is the data source configuration.
type DataSource struct {
	// BackupName is the name of the backup from backup location to use
	BackupName string `json:"backupName"`
	// ObjectStorageName is the name of the ObjectStorage CR that defines the
	// storage location
	ObjectStorageName string `json:"objectStorageName"`
}

// BackupSchedule is the backup schedule configuration.
type BackupSchedule struct {
	// Enabled is a flag to enable the schedule
	Enabled bool `json:"enabled"`
	// Name is the name of the schedule
	Name string `json:"name"`
	// RetentionCopies is the number of backup copies to retain
	RetentionCopies int32 `json:"retentionCopies,omitempty"`
	// Schedule is the cron schedule
	Schedule string `json:"schedule"`
	// ObjectStorageName is the name of the ObjectStorage CR that defines the
	// storage location
	ObjectStorageName string `json:"objectStorageName"`
}

// Backup is the backup configuration.
type Backup struct {
	// Enabled is a flag to enable backups
	Enabled bool `json:"enabled"`
	// Schedules is a list of backup schedules
	Schedules []BackupSchedule `json:"schedules,omitempty"`
}

// PMMSpec contains PMM settings.
type PMMSpec struct {
	Image         string `json:"image,omitempty"`
	ServerHost    string `json:"serverHost,omitempty"`
	ServerUser    string `json:"serverUser,omitempty"`
	PublicAddress string `json:"publicAddress,omitempty"`
	Login         string `json:"login,omitempty"`
	Password      string `json:"password,omitempty"`
}

// Monitoring contains monitoring settings.
type Monitoring struct {
	// Enabled is a flag to enable monitoring
	Enabled   bool                        `json:"enabled"`
	PMM       *PMMSpec                    `json:"pmm,omitempty"`
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
	//nolint:godox,dupword,gocritic
	// TODO migrate to the MonitoringConfig CR approach.
	//// MonitoringConfigName is the name of the MonitoringConfig CR that defines
	//// the monitoring configuration
	//MonitoringConfigName string `json:"monitoringConfigName,omitempty"`
	//// Resources are the resource requirements for the monitoring container.
	//// If not set, resource limits are not imposed
	//Resources Resources `json:"resources,omitempty"`
}

// DatabaseClusterSpec defines the desired state of DatabaseCluster.
type DatabaseClusterSpec struct {
	// Paused is a flag to stop the cluster
	Paused bool `json:"paused,omitempty"`
	// Engine is the database engine specification
	Engine Engine `json:"engine"`
	// Proxy is the proxy specification. If not set, an appropriate
	// proxy specification will be applied for the given engine. A
	// common use case for setting this field is to control the
	// external access to the database cluster.
	Proxy Proxy `json:"proxy,omitempty"`
	// AdminUserSecretName is the name of the secret that contains the admin
	// user credentials
	AdminUserSecretName string `json:"adminSecretName,omitempty"`
	// DataSource defines a data source for bootstraping a new cluster
	DataSource *DataSource `json:"dataSource,omitempty"`
	// Backup is the backup specification
	Backup Backup `json:"backup,omitempty"`
	// Monitoring is the monitoring specification
	Monitoring Monitoring `json:"monitoring,omitempty"`
}

// DatabaseClusterStatus defines the observed state of DatabaseCluster.
type DatabaseClusterStatus struct {
	// Status is the status of the cluster
	Status AppState `json:"status,omitempty"`
	// Hostname is the hostname where the cluster can be reached
	Hostname string `json:"hostname,omitempty"`
	// Port is the port where the cluster can be reached
	Port int32 `json:"port,omitempty"`
	// Ready is the number of ready pods
	Ready int32 `json:"ready,omitempty"`
	// Size is the total number of pods
	Size int32 `json:"size,omitempty"`
	// Message is extra information about the cluster
	Message string `json:"message,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
// +kubebuilder:resource:shortName=db;dbc
// +kubebuilder:printcolumn:name="Size",type="string",JSONPath=".status.size"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.ready"
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.status"
// +kubebuilder:printcolumn:name="Hostname",type="string",JSONPath=".status.hostname"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// DatabaseCluster is the Schema for the databaseclusters API.
type DatabaseCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DatabaseClusterSpec   `json:"spec,omitempty"`
	Status DatabaseClusterStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// DatabaseClusterList contains a list of DatabaseCluster.
type DatabaseClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DatabaseCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DatabaseCluster{}, &DatabaseClusterList{})
}
