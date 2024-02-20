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

// MonitoringType is a type of monitoring.
type MonitoringType string

const (
	// PMMMonitoringType represents monitoring via PMM.
	PMMMonitoringType MonitoringType = "pmm"
	// MonitoringConfigCredentialsSecretUsernameKey is the credentials secret's key that contains the username.
	MonitoringConfigCredentialsSecretUsernameKey = "username"
	// MonitoringConfigCredentialsSecretAPIKeyKey is the credentials secret's key that contains the API key.
	MonitoringConfigCredentialsSecretAPIKeyKey = "apiKey"
)

// MonitoringConfigSpec defines the desired state of MonitoringConfig.
type MonitoringConfigSpec struct {
	// Type is type of monitoring.
	// +kubebuilder:validation:Enum=pmm
	Type MonitoringType `json:"type"`
	// CredentialsSecretName is the name of the secret with credentials.
	CredentialsSecretName string `json:"credentialsSecretName"`
	// AllowedNamespaces is the list of namespaces where the operator will copy secrets provided in the CredentialsSecretsName.
	AllowedNamespaces []string `json:"allowedNamespaces,omitempty"`
	// PMM is configuration for the PMM monitoring type.
	PMM PMMConfig `json:"pmm,omitempty"`
}

// PMMConfig is configuration of the PMM monitoring type.
type PMMConfig struct {
	// URL is url to the monitoring instance.
	URL string `json:"url"`
	// Image is a Docker image name to use for deploying PMM client. Defaults to using the latest version.
	Image string `json:"image"`
}

// MonitoringConfigStatus defines the observed state of MonitoringConfig.
type MonitoringConfigStatus struct{}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:printcolumn:name="Type",type="string",JSONPath=".spec.type",description="Monitoring instance type"

// MonitoringConfig is the Schema for the monitoringconfigs API.
type MonitoringConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MonitoringConfigSpec   `json:"spec,omitempty"`
	Status MonitoringConfigStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// MonitoringConfigList contains a list of MonitoringConfig.
type MonitoringConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MonitoringConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&MonitoringConfig{}, &MonitoringConfigList{})
}

// IsNamespaceAllowed checks the namespace against allowedNamespaces and returns if it's allowed to use.
func (m *MonitoringConfig) IsNamespaceAllowed(namespace string) bool {
	if len(m.Spec.AllowedNamespaces) == 0 {
		return true
	}
	for _, ns := range m.Spec.AllowedNamespaces {
		ns := ns
		if ns == namespace {
			return true
		}
	}
	return false
}
