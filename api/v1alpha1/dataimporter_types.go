package v1alpha1

import (
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:shortName=di
//+kubebuilder:printcolumn:name="DisplayName",type="string",JSONPath=".spec.displayName"
//+kubebuilder:printcolumn:name="Description",type="string",JSONPath=".spec.description"
//+kubebuilder:printcolumn:name="SupportedEngines",type="string",JSONPath=".spec.supportedEngines"
//+kubebuilder:resource:scope=Cluster

// DataImporter defines a reusable strategy for importing data into a DatabaseCluster.
type DataImporter struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DataImporterSpec   `json:"spec,omitempty"`
	Status DataImporterStatus `json:"status,omitempty"`
}

// DataImporterSpec defines the specification of a DataImporter.
type DataImporterSpec struct {
	// DisplayName is a human-readable name for the data importer.
	DisplayName string `json:"displayName,omitempty"`
	// Description is the description of the data importer.
	Description string `json:"description,omitempty"`
	// SupportedEngines is the list of engines that the data importer supports.
	SupportedEngines []EngineType `json:"supportedEngines,omitempty"`
	// Config contains additional configuration defined for the data importer.
	Config DataImporterConfig `json:"config,omitempty"`
	// JobSpec is the specification of the data importer job.
	JobSpec DataImporterJobSpec `json:"jobSpec,omitempty"`
	// Permissions is the list of permissions that the data importer needs.
	Permissions []rbacv1.PolicyRule `json:"permissions,omitempty"`
	// ClusterPermissions is the list of cluster-wide permissions that the data importer needs.
	ClusterPermissions []rbacv1.PolicyRule `json:"clusterPermissions,omitempty"`
}

// DataImporterConfig contains additional configuration defined for the data importer.
type DataImporterConfig struct {
	// OpenAPIV3Schema is the OpenAPI v3 schema of the data importer.
	// +kubebuilder:pruning:PreserveUnknownFields
	// +kubebuilder:validation:Schemaless
	// +optional
	OpenAPIV3Schema *apiextensionsv1.JSONSchemaProps `json:"openAPIV3Schema,omitempty"`
}

type DataImporterJobSpec struct {
	// Image is the image of the data importer.
	Image string `json:"image,omitempty"`
	// Command is the command to run the data importer.
	// +optional
	Command []string `json:"command,omitempty"`
}

// +kubebuilder:object:root=true
// DataImporterList contains a list of DataImporter.
type DataImporterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DataImporter `json:"items"`
}

type DataImporterStatus struct{}

func init() {
	SchemeBuilder.Register(&DataImporter{}, &DataImporterList{})
}
