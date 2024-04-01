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

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status

// ComponentsUpgrade is the Schema for the componentupgrades API.
type ComponentsUpgrade struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ComponentsUpgradeSpec   `json:"spec,omitempty"`
	Status ComponentsUpgradeStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// ComponentsUpgradeList contains a list of ComponentsUpgrade.
type ComponentsUpgradeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BackupStorage `json:"items"`
}

type ComponentsUpgradeSpec struct {
	// LockNamespace specifies if the namespace should be locked during the upgrade.
	LockNamespace *bool `json:"lockNamespace,omitempty"`
	// TimeoutMinutes specifies the timeout for the upgrade process.
	// +kubebuilder:default:=15
	TimeoutMinutes *int32 `json:"timeoutMinutes,omitempty"`

	PGUpgrade     *PGOUpgradeSpec    `json:"pg,omitempty"`
	PSMDBOUpgrade *PSMDBOUpgradeSpec `json:"psmdb,omitempty"`
	PXCOUpgrade   *PXCOUpgradeSpec   `json:"pxc,omitempty"`
}

type PGOUpgradeSpec struct {
	TargetVersion string `json:"targetVersion,omitempty"`
}

type PSMDBOUpgradeSpec struct {
	TargetVersion string `json:"targetVersion,omitempty"`
}

type PXCOUpgradeSpec struct {
	TargetVersion string `json:"targetVersion,omitempty"`
}

const (
	ComponentsUpgradeConditionTypeStarted  = "Started"
	ComponentsUpgradeConditionTypeFailed   = "Failed"
	ComponentsUpgradeConditionTypeTimedOut = "TimedOut"
	ComponentsUpgradeConditionTypeSuccess  = "Success"
	ComponentsUpgradeConditionTypeNsLocked = "NamespaceLocked"
)

type ComponentsUpgradeStatus struct {
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

func init() {
	SchemeBuilder.Register(&ComponentsUpgrade{}, &ComponentsUpgradeList{})
}
