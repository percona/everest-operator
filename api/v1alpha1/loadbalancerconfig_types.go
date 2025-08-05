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

// LoadBalancerConfigSpec defines the desired state of LoadBalancerConfig.
type LoadBalancerConfigSpec struct {
	// Annotations key-value pairs to apply as annotations to the load balancer
	// +kubebuilder:validation:XValidation:message="Invalid annotation key: must conform to DNS subdomain (e.g., 'example.com/key') or start with 'kubernetes.io/' or 'k8s.io/'", rule="self.all(key, key.matches('^([a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*\\/)?[a-z0-9]([-a-z0-9]*[a-z0-9])?$') || key.startsWith('kubernetes.io/') || key.startsWith('k8s.io/'))"
	Annotations map[string]string `json:"annotations,omitempty"`
}

// LoadBalancerConfigStatus defines the observed state of LoadBalancerConfig.
type LoadBalancerConfigStatus struct {
	// InUse is a flag that indicates if the config is used by any DB cluster.
	// +kubebuilder:default=false
	InUse bool `json:"inUse,omitempty"`
	// LastObservedGeneration is the most recent generation observed for this LoadBalancerConfig.
	LastObservedGeneration int64 `json:"lastObservedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster,path=loadbalancerconfigs,shortName=lbc
// +kubebuilder:printcolumn:name="InUse",type="string",JSONPath=".status.inUse",description="Indicates if the config is used by any DB cluster"

// LoadBalancerConfig is the Schema for the Load Balancer Config API.
type LoadBalancerConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec LoadBalancerConfigSpec `json:"spec,omitempty"`
	// +kubebuilder:default={"inUse": false}
	Status LoadBalancerConfigStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// LoadBalancerConfigList contains a list of LoadBalancerConfig.
type LoadBalancerConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []LoadBalancerConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&LoadBalancerConfig{}, &LoadBalancerConfigList{})
}
