// everest-operator
// Copyright (C) 2025 Percona LLC
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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type PXCAffinityConfig struct {
	// Engine is the affinity configuration for the DB Engine pods.
	Engine *corev1.Affinity `json:"engine,omitempty"`
	// Proxy is the affinity configuration for the DB Proxy pods.
	Proxy *corev1.Affinity `json:"proxy,omitempty"`
}
type PostgreSQLAffinityConfig struct {
	// Engine is the affinity configuration for the DB Engine pods.
	Engine *corev1.Affinity `json:"engine,omitempty"`
	// Proxy is the affinity configuration for the DB Proxy pods.
	Proxy *corev1.Affinity `json:"proxy,omitempty"`
}

type PSMDBAffinityConfig struct {
	// Engine is the affinity configuration for the DB Engine pods.
	Engine *corev1.Affinity `json:"engine,omitempty"`
	// Proxy is the affinity configuration for the DB Proxy pods.
	Proxy *corev1.Affinity `json:"proxy,omitempty"`
	// ConfigServer is the affinity configuration for the DB Config Server pods.
	ConfigServer *corev1.Affinity `json:"configServer,omitempty"`
}

// AffinityConfig is a configuration for the affinity settings depending on the engine type.
// Only one of the fields should be set.
type AffinityConfig struct {
	// PXC is the affinity configuration for the PXC DB clusters.
	PXC *PXCAffinityConfig `json:"pxc,omitempty"`
	// PostgreSQL is the affinity configuration for the PostgreSQL DB clusters.
	PostgreSQL *PostgreSQLAffinityConfig `json:"postgresql,omitempty"`
	// PSMDB is the affinity configuration for the PSMDB DB clusters.
	PSMDB *PSMDBAffinityConfig `json:"psmdb,omitempty"`
}

// PodSchedulingPolicySpec defines the desired state of PodSchedulingPolicy.
type PodSchedulingPolicySpec struct {
	// EngineType is type of DB engine that this policy can be applied to.
	// +kubebuilder:validation:Enum=pxc;postgresql;psmdb
	EngineType EngineType `json:"engineType"`
	// AffinityConfig is a configuration for the affinity settings depending on the engine type.
	AffinityConfig *AffinityConfig `json:"affinityConfig,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,path=podschedulingpolicies,shortName=psp
// +kubebuilder:selectablefield:JSONPathstring=".spec.engineType"
// +kubebuilder:printcolumn:name="Engine",type="string",JSONPath=".spec.engineType",description="DB engine type the policy can be applied to"

// PodSchedulingPolicy is the Schema for the Pod Scheduling Policy API.
type PodSchedulingPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec PodSchedulingPolicySpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// PodSchedulingPolicyList contains a list of PodSchedulingPolicy.
type PodSchedulingPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []PodSchedulingPolicy `json:"items"`
}

func init() {
	SchemeBuilder.Register(&PodSchedulingPolicy{}, &PodSchedulingPolicyList{})
}
