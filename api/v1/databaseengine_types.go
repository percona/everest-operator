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

const (
	// DBEngineStateNotInstalled represents the state of engine when underlying operator is not installed.
	DBEngineStateNotInstalled EngineState = "not installed"
	// DBEngineStateInstalling represents the state of engine when underlying operator is installing.
	DBEngineStateInstalling EngineState = "installing"
	// DBEngineStateInstalled represents the state of engine when underlying operator is installed.
	DBEngineStateInstalled EngineState = "installed"
)

type (
	// EngineType stands for the supported database engines. Right now it's only pxc
	// and psmdb. However, it can be ps, pg and any other source.
	EngineType string

	// EngineState represents state of engine in a k8s cluster.
	EngineState string
)

// DatabaseEngineSpec is a spec for a database engine.
type DatabaseEngineSpec struct {
	Type EngineType `json:"type"`
}

// DatabaseEngineStatus defines the observed state of DatabaseEngine.
type DatabaseEngineStatus struct {
	State   EngineState `json:"status,omitempty"`
	Version string      `json:"version,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:shortName=dbengine;
//+kubebuilder:printcolumn:name="Type",type="string",JSONPath=".spec.type"
//+kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.status"
//+kubebuilder:printcolumn:name="Version",type="string",JSONPath=".status.version"

// DatabaseEngine is the Schema for the databaseengines API.
type DatabaseEngine struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DatabaseEngineSpec   `json:"spec,omitempty"`
	Status DatabaseEngineStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// DatabaseEngineList contains a list of DatabaseEngine.
type DatabaseEngineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DatabaseEngine `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DatabaseEngine{}, &DatabaseEngineList{})
}
