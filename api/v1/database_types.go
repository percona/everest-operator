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
	"github.com/percona/percona-backup-mongodb/pbm/compress"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// PXCEngine represents engine type for PXC clusters.
	PXCEngine EngineType = "pxc"
	// PSMDBEngine represents engine type for PSMDB clusters.
	PSMDBEngine EngineType = "psmdb"
	// LoadBalancerMongos represents mongos load balancer.
	LoadBalancerMongos LoadBalancerType = "mongos"
	// LoadBalancerHAProxy represents haproxy load balancer.
	LoadBalancerHAProxy LoadBalancerType = "haproxy"
	// LoadBalancerProxySQL represents proxySQL load balancer.
	LoadBalancerProxySQL LoadBalancerType = "proxysql"
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
)

type (
	// LoadBalancerType contains supported loadbalancers. It can be proxysql or haproxy
	// for PXC clusters and mongos for PSMDB clusters.
	//
	// Once PG support will be added, it can be pg-bouncer or something else.
	LoadBalancerType string
	// AppState is used to represent cluster's state.
	AppState string
	// DatabaseSpec defines the desired state of Database.
	DatabaseSpec struct {
		// Database type stands for supported databases by the PMM API
		// Now it's pxc or psmdb types but we can extend it.
		Database EngineType `json:"databaseType"`
		// DatabaseVersion sets from version service and uses the recommended version
		// by default.
		DatabaseImage string `json:"databaseImage"`
		// DatabaseConfig contains a config settings for the specified database.
		DatabaseConfig string `json:"databaseConfig"`
		// SecretsName contains name of a secrets file for a database cluster.
		SecretsName string `json:"secretsName,omitempty"`
		// Pause represents is a cluster paused or not.
		Pause bool `json:"pause,omitempty"`
		// ClusterSize is amount of nodes that required for the cluster.
		// A database starts in cluster mode if clusterSize >= 3.
		ClusterSize int32 `json:"clusterSize"`
		// LoadBalancer contains a load balancer settings. For PXC it's haproxy
		// or proxysql. For PSMDB it's mongos.
		LoadBalancer LoadBalancerSpec `json:"loadBalancer,omitempty"`
		// Monitoring contains a monitoring settings.
		Monitoring MonitoringSpec `json:"monitoring,omitempty"`
		// DBInstance represents resource requests for a database cluster.
		DBInstance DBInstanceSpec `json:"dbInstance"`
		// Backup contains backup settings.
		Backup *BackupSpec `json:"backup,omitempty"`
	}
	// LoadBalancerSpec contains a load balancer settings. For PXC it's haproxy
	// or proxysql. For PSMDB it's mongos.
	LoadBalancerSpec struct {
		Type                     LoadBalancerType                        `json:"type,omitempty"`
		ExposeType               corev1.ServiceType                      `json:"exposeType,omitempty"`
		Image                    string                                  `json:"image,omitempty"`
		Size                     int32                                   `json:"size,omitempty"`
		Configuration            string                                  `json:"configuration,omitempty"`
		LoadBalancerSourceRanges []string                                `json:"loadBalancerSourceRanges,omitempty"`
		Annotations              map[string]string                       `json:"annotations,omitempty"`
		TrafficPolicy            corev1.ServiceExternalTrafficPolicyType `json:"trafficPolicy,omitempty"`
		Resources                corev1.ResourceRequirements             `json:"resources,omitempty"`
	}
	// MonitoringSpec contains monitoring settings.
	MonitoringSpec struct {
		PMM                      *PMMSpec                    `json:"pmm,omitempty"`
		ImagePullPolicy          corev1.PullPolicy           `json:"imagePullPolicy,omitempty"`
		Resources                corev1.ResourceRequirements `json:"resources,omitempty"`
		RuntimeClassName         *string                     `json:"runtimeClassName,omitempty"`
		ContainerSecurityContext *corev1.SecurityContext     `json:"containerSecurityContext,omitempty"`
	}
	// PMMSpec contains PMM settings.
	PMMSpec struct {
		Image         string `json:"image,omitempty"`
		ServerHost    string `json:"serverHost,omitempty"`
		ServerUser    string `json:"serverUser,omitempty"`
		PublicAddress string `json:"publicAddress,omitempty"`
		Login         string `json:"login,omitempty"`
		Password      string `json:"password,omitempty"`
	}
	// DBInstanceSpec represents resource requests for database cluster.
	DBInstanceSpec struct {
		CPU              resource.Quantity `json:"cpu,omitempty"`
		Memory           resource.Quantity `json:"memory,omitempty"`
		DiskSize         resource.Quantity `json:"diskSize,omitempty"`
		StorageClassName *string           `json:"storageClassName,omitempty"`
	}
	// BackupSpec contains backup settings.
	BackupSpec struct {
		Enabled                  bool                          `json:"enabled,omitempty"`
		Image                    string                        `json:"image,omitempty"`
		InitImage                string                        `json:"initImage,omitempty"`
		ImagePullSecrets         []corev1.LocalObjectReference `json:"imagePullSecrets,omitempty"`
		ImagePullPolicy          corev1.PullPolicy             `json:"imagePullPolicy,omitempty"`
		Schedule                 []BackupSchedule              `json:"schedule,omitempty"`
		ServiceAccountName       string                        `json:"serviceAccountName,omitempty"`
		ContainerSecurityContext *corev1.SecurityContext       `json:"containerSecurityContext,omitempty"`
		Resources                corev1.ResourceRequirements   `json:"resources,omitempty"`
		Storages                 map[string]*BackupStorageSpec `json:"storages,omitempty"`
		Annotations              map[string]string             `json:"annotations,omitempty"`
		Labels                   map[string]string             `json:"labels,omitempty"`
	}
	// BackupSchedule represents set of settings to configure backup schedule.
	BackupSchedule struct {
		Name             string                   `json:"name,omitempty"`
		Enabled          bool                     `json:"enabled,omitempty"`
		Schedule         string                   `json:"schedule,omitempty"`
		Keep             int                      `json:"keep,omitempty"`
		StorageName      string                   `json:"storageName,omitempty"`
		CompressionType  compress.CompressionType `json:"compressionType,omitempty"`
		CompressionLevel *int                     `json:"compressionLevel,omitempty"`
	}
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
	// BackupStorageSpec represents set of settings to configure backup storage.
	BackupStorageSpec struct {
		Type                     BackupStorageType           `json:"type"`
		Volume                   *VolumeSpec                 `json:"volumeSpec,omitempty"`
		StorageProvider          *BackupStorageProviderSpec  `json:"storageProvider,omitempty"`
		NodeSelector             map[string]string           `json:"nodeSelector,omitempty"`
		Resources                corev1.ResourceRequirements `json:"resources,omitempty"`
		Affinity                 *corev1.Affinity            `json:"affinity,omitempty"`
		Tolerations              []corev1.Toleration         `json:"tolerations,omitempty"`
		Annotations              map[string]string           `json:"annotations,omitempty"`
		Labels                   map[string]string           `json:"labels,omitempty"`
		SchedulerName            string                      `json:"schedulerName,omitempty"`
		PriorityClassName        string                      `json:"priorityClassName,omitempty"`
		PodSecurityContext       *corev1.PodSecurityContext  `json:"podSecurityContext,omitempty"`
		ContainerSecurityContext *corev1.SecurityContext     `json:"containerSecurityContext,omitempty"`
		RuntimeClassName         *string                     `json:"runtimeClassName,omitempty"`
		VerifyTLS                *bool                       `json:"verifyTLS,omitempty"`
	}
	// VolumeSpec represents a specification to configure volume for underlying database.
	VolumeSpec struct {
		// EmptyDir to use as data volume for mysql. EmptyDir represents a temporary
		// directory that shares a pod's lifetime.
		// +optional
		EmptyDir *corev1.EmptyDirVolumeSource `json:"emptyDir,omitempty"`

		// HostPath to use as data volume for mysql. HostPath represents a
		// pre-existing file or directory on the host machine that is directly
		// exposed to the container.
		// +optional
		HostPath *corev1.HostPathVolumeSource `json:"hostPath,omitempty"`

		// PersistentVolumeClaim to specify PVC spec for the volume for mysql data.
		// It has the highest level of precedence, followed by HostPath and
		// EmptyDir. And represents the PVC specification.
		// +optional
		PersistentVolumeClaim *corev1.PersistentVolumeClaimSpec `json:"persistentVolumeClaim,omitempty"`
	}
)

// DatabaseClusterStatus defines the observed state of Database.
type DatabaseClusterStatus struct {
	Ready   int32    `json:"ready,omitempty"`
	Size    int32    `json:"size,omitempty"`
	State   AppState `json:"status,omitempty"`
	Host    string   `json:"host,omitempty"`
	Message string   `json:"message,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
// +kubebuilder:resource:shortName=db;dbc
// +kubebuilder:printcolumn:name="Size",type="string",JSONPath=".status.size"
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.ready"
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.status"
// +kubebuilder:printcolumn:name="ENDPOINT",type="string",JSONPath=".status.host"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// DatabaseCluster is the Schema for the databases API.
type DatabaseCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DatabaseSpec          `json:"spec,omitempty"`
	Status DatabaseClusterStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// DatabaseClusterList contains a list of Database.
type DatabaseClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DatabaseCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DatabaseCluster{}, &DatabaseClusterList{})
}
