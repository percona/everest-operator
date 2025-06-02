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
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:shortName=dij
//+kubebuilder:printcolumn:name="TargetCluster",type="string",JSONPath=".spec.targetClusterName"
//+kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"

// DataImportJob is the schema for the dataimportjobs API.
type DataImportJob struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DataImportJobSpec   `json:"spec,omitempty"`
	Status DataImportJobStatus `json:"status,omitempty"`
}

type DataImportJobSpec struct {
	// TargetClusterName is the reference to the target cluster.
	// +kubebuilder:validation:Required
	TargetClusterName      string `json:"targetClusterName,omitempty"`
	*DataImportJobTemplate `json:",inline"`
}

// DataImportJobTemplate defines a shared template for the data import job.
type DataImportJobTemplate struct {
	// DataImporterName is the data importer to use for the import.
	// +kubebuilder:validation:Required
	DataImporterName string `json:"dataImporterName,omitempty"`
	// Source is the source of the data to import.
	// +kubebuilder:validation:Required
	Source *DataImportJobSource `json:"source,omitempty"`
	// Config defines the configuration for the data import job.
	// These options are specific to the DataImporter being used and must conform to
	// the schema defined in the DataImporter's .spec.config.openAPIV3Schema.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +optional
	Config *runtime.RawExtension `json:"config,omitempty"`
}

type DataImportJobSource struct {
	// S3 contains the S3 information for the data import.
	// +optional
	S3 *DataImportJobS3Source `json:"s3,omitempty"`
	// Path is the path to the directory to import the data from.
	// This may be a path to a file or a directory, depending on the data importer.
	// +optional
	Path string `json:"path,omitempty"`
}

type DataImportJobS3Source struct {
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
	// CredentialsSecreName is the reference to the secret containing the S3 credentials.
	// The Secret must contain the keys `AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY`.
	// +kubebuilder:validation:Required
	CredentialsSecretName string `json:"credentialsSecretName,omitempty"`
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
	// StartedAt is the time when the data import job started.
	StartedAt *metav1.Time `json:"startedAt,omitempty"`
	// LastObservedGeneration is the last observed generation of the data import job.
	LastObservedGeneration int64 `json:"lastObservedGeneration,omitempty"`
	// Phase is the current phase of the data import job.
	Phase DataImportJobPhase `json:"phase,omitempty"`
	// Message is the message of the data import job.
	Message string `json:"message,omitempty"`
	// JobName is the reference to the job that is running the data import.
	// +optional
	JobName string `json:"jobName,omitempty"`
}

// Equals compares two DataImportJobSpec objects for equality.
func (s *DataImportJobSpec) Equals(other DataImportJobSpec) bool {
	return reflect.DeepEqual(s, other)
}

// Equals compares two DataImportJobTemplate objects for equality.
func (s *DataImportJobTemplate) Equals(other DataImportJobTemplate) bool {
	return reflect.DeepEqual(s, other)
}

func init() {
	SchemeBuilder.Register(&DataImportJob{}, &DataImportJobList{})
}
