package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:shortName=dij
//+kubebuilder:printcolumn:name="TargetCluster",type="string",JSONPath=".spec.targetClusterRef.name"
//+kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"

// DataImportJob is the schema for the dataimportjobs API.
type DataImportJob struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DataImportJobSpec   `json:"spec,omitempty"`
	Status DataImportJobStatus `json:"status,omitempty"`
}

type DataImportJobSpec struct {
	// TargetClusterRef is the reference to the target cluster.
	TargetClusterRef corev1.LocalObjectReference `json:"targetClusterRef,omitempty"`
	// DataImporterRef is the reference to the data importer.
	DataImporterRef corev1.LocalObjectReference `json:"dataImporterRef,omitempty"`
	// Source is the source of the data to import.
	Source DataImportJobSource `json:"source,omitempty"`
	// Params defines the configuration parameters for the data import job.
	// These parameters are specific to the DataImporter being used and must conform to
	// the schema defined in the DataImporter's .spec.config.openAPIV3Schema.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	Params *runtime.RawExtension `json:"parameters,omitempty"`
	// ServiceAccountName is the name of the service account to use for the import job.
	// This service account must have the necessary permissions to perform the import.
	// +optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`
}

type DataImportJobSource struct {
	// BackupStorageRef is the reference to the backup storage.
	// +optional
	BackupStorageRef corev1.LocalObjectReference `json:"backupStorageRef,omitempty"`
	// S3Ref is the reference to the S3 source.
	// +optional
	S3Ref *DataImportJobS3SourceRef `json:"s3,omitempty"`
	// Path is the path to the directory to import the data from.
	// This may be a path to a file or a directory, depending on the data importer.
	// +optional
	Path string `json:"path,omitempty"`
}

type DataImportJobS3SourceRef struct {
	// Bucket is the name of the S3 bucket.
	Bucket string `json:"bucket,omitempty"`
	// Region is the region of the S3 bucket.
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
	// CredentialsSecretRef is the reference to the secret containing the S3 credentials.
	// The Secret must contain the keys `AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY`.
	CredentialsSecretRef corev1.LocalObjectReference `json:"credentialsSecretRef,omitempty"`
}

// +kubebuilder:object:root=true
type DataImportJobList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DataImportJob `json:"items"`
}

type DataImportJobPhase string

const (
	DataImportJobPhasePending   DataImportJobPhase = "Pending"
	DataImportJobPhaseRunning   DataImportJobPhase = "Running"
	DataImportJobPhaseCompleted DataImportJobPhase = "Completed"
	DataImportJobPhaseFailed    DataImportJobPhase = "Failed"
	DataImportJobPhaseError     DataImportJobPhase = "Error"
)

type DataImportJobStatus struct {
	// Phase is the current phase of the data import job.
	Phase DataImportJobPhase `json:"phase,omitempty"`
	// Message is the message of the data import job.
	Message string `json:"message,omitempty"`
	// JobRef is the reference to the job that is running the data import.
	// +optional
	JobRef *corev1.LocalObjectReference `json:"jobRef,omitempty"`
}

func init() {
	SchemeBuilder.Register(&DataImportJob{}, &DataImportJobList{})
}
