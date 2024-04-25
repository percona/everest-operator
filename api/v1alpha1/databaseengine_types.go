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
	"sort"

	goversion "github.com/hashicorp/go-version"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// DBEngineStateNotInstalled represents the state of engine when underlying operator is not installed.
	DBEngineStateNotInstalled EngineState = "not installed"
	// DBEngineStateInstalling represents the state of engine when underlying operator is installing.
	DBEngineStateInstalling EngineState = "installing"
	// DBEngineStateInstalled represents the state of engine when underlying operator is installed.
	DBEngineStateInstalled EngineState = "installed"
	// DBEngineStateUpgrading represents the state of engine when underlying operator is upgrading.
	DBEngineStateUpgrading EngineState = "upgrading"

	// DatabaseEnginePXC represents engine type for PXC clusters.
	DatabaseEnginePXC EngineType = "pxc"
	// DatabaseEnginePSMDB represents engine type for PSMDB clusters.
	DatabaseEnginePSMDB EngineType = "psmdb"
	// DatabaseEnginePostgresql represents engine type for Postgresql clusters.
	DatabaseEnginePostgresql EngineType = "postgresql"

	// DBEngineComponentRecommended represents recommended component status.
	DBEngineComponentRecommended ComponentStatus = "recommended"
	// DBEngineComponentAvailable represents available component status.
	DBEngineComponentAvailable ComponentStatus = "available"
	// DBEngineComponentUnavailable represents unavailable component status.
	DBEngineComponentUnavailable ComponentStatus = "unavailable"
	// DBEngineComponentUnsupported represents unsupported component status.
	DBEngineComponentUnsupported ComponentStatus = "unsupported"
)

const (
	// DatabaseOperatorUpgradeAnnotation indicates that the database operator needs to be upgraded.
	// The value of the annotation is the version to upgrade to.
	DatabaseOperatorUpgradeAnnotation = "everest.percona.com/upgrade-operator-to"
	// DatabaseOperatorUpgradeLockAnnotation is an annotation set on the database engine.
	// If present and set to "true", Everest backend will prevent any resources from being modified in the namespace.
	DatabaseOperatorUpgradeLockAnnotation = "everest.percona.com/upgrade-lock"
	// DatabaseOperatorUpgradeLockAnnotationValueTrue is the value of the DatabaseOperatorUpgradeLockAnnotation
	// if the database engine must be locked.
	DatabaseOperatorUpgradeLockAnnotationValueTrue = "true"
)

type (
	// EngineType stands for the supported database engines. Right now it's only pxc
	// and psmdb. However, it can be ps, pg and any other source.
	EngineType string

	// EngineState represents state of engine in a k8s cluster.
	EngineState string

	// UpgradePhase represents the phase of the operator upgrade.
	UpgradePhase string
)

// DatabaseEngineSpec is a spec for a database engine.
type DatabaseEngineSpec struct {
	Type            EngineType `json:"type"`
	AllowedVersions []string   `json:"allowedVersions,omitempty"`
}

// DatabaseEngineStatus defines the observed state of DatabaseEngine.
type DatabaseEngineStatus struct {
	State                   EngineState       `json:"status,omitempty"`
	OperatorVersion         string            `json:"operatorVersion,omitempty"`
	AvailableVersions       Versions          `json:"availableVersions,omitempty"`
	PendingOperatorUpgrades []OperatorUpgrade `json:"pendingOperatorUpgrades,omitempty"`

	// OperatorUpgrade contains the status of the operator upgrade.
	// This is set only if the `everest.percona.com/upgrade-operator-to` annotation is present.
	OperatorUpgrade *OperatorUpgradeStatus `json:"operatorUpgrade,omitempty"`
}

// OperatorUpgrade contains the information about the operator upgrade.
type OperatorUpgrade struct {
	// TargetVersion is the version to which the operator should be upgraded.
	TargetVersion string `json:"targetVersion,omitempty"`
	// InstallPlanRef is a reference to the InstallPlan object created for the operator upgrade.
	InstallPlanRef corev1.LocalObjectReference `json:"installPlanRef,omitempty"`
}

// GetPendingUpgrade gets a reference to the pending OperatorUpgrade for the given targetVersion.
func (s *DatabaseEngineStatus) GetPendingUpgrade(targetVersion string) *OperatorUpgrade {
	for _, upgrade := range s.PendingOperatorUpgrades {
		if upgrade.TargetVersion == targetVersion {
			return &upgrade
		}
	}
	return nil
}

const (
	// UpgradePhaseStarted represents the phase when the operator upgrade has started.
	UpgradePhaseStarted UpgradePhase = "started"
	// UpgradePhaseCompleted represents the phase when the operator upgrade has completed.
	UpgradePhaseCompleted UpgradePhase = "completed"
	// UpgradePhaseFailed represents the phase when the operator upgrade has failed.
	UpgradePhaseFailed UpgradePhase = "failed"
)

// OperatorUpgradeStatus contains the status of the operator upgrade.
type OperatorUpgradeStatus struct {
	OperatorUpgrade `json:",inline"`
	Phase           UpgradePhase `json:"phase,omitempty"`
	StartedAt       *metav1.Time `json:"startedAt,omitempty"`
	Message         string       `json:"message,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:resource:shortName=dbengine;
//+kubebuilder:printcolumn:name="Type",type="string",JSONPath=".spec.type"
//+kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.status"
//+kubebuilder:printcolumn:name="Operator Version",type="string",JSONPath=".status.operatorVersion"

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

// Versions struct represents available versions of database engine components.
type Versions struct {
	Engine ComponentsMap               `json:"engine,omitempty"`
	Backup ComponentsMap               `json:"backup,omitempty"`
	Proxy  map[ProxyType]ComponentsMap `json:"proxy,omitempty"`
	Tools  map[string]ComponentsMap    `json:"tools,omitempty"`
}

// ComponentsMap is a map of database engine components.
type ComponentsMap map[string]*Component

// ComponentStatus represents status of the database engine component.
type ComponentStatus string

// Component contains information of the database engine component.
// Database Engine component can be database engine, database proxy or tools image path.
type Component struct {
	Critical  bool            `json:"critical,omitempty"`
	ImageHash string          `json:"imageHash,omitempty"`
	ImagePath string          `json:"imagePath,omitempty"`
	Status    ComponentStatus `json:"status,omitempty"`
}

// FilterStatus returns a new ComponentsMap with components filtered by status.
func (c ComponentsMap) FilterStatus(statuses ...ComponentStatus) ComponentsMap {
	result := make(ComponentsMap)
	for version, component := range c {
		for _, status := range statuses {
			if component.Status == status {
				result[version] = component
			}
		}
	}
	return result
}

// GetSortedVersions returns a sorted slice of versions. Versions are sorted in
// descending order. Most recent version is first.
func (c ComponentsMap) GetSortedVersions() []string {
	versions := make(goversion.Collection, 0, len(c))
	for version := range c {
		v, err := goversion.NewVersion(version)
		if err != nil {
			continue
		}
		versions = append(versions, v)
	}
	sort.Sort(versions)

	// Reverse order and return the original version strings.
	result := make([]string, 0, len(versions))
	for i := len(versions) - 1; i >= 0; i-- {
		result = append(result, versions[i].Original())
	}

	return result
}

// GetAllowedVersionsSorted returns a sorted slice of allowed versions.
// An allowed version is a version whose status is either recommended or
// available. Allowed versions are sorted by status, with recommended versions
// first, followed by available versions. Versions with the same status are
// sorted by version in descending order. Most recent version is first.
func (c ComponentsMap) GetAllowedVersionsSorted() []string {
	recommendedComponents := c.FilterStatus(DBEngineComponentRecommended)
	recommendedVersions := recommendedComponents.GetSortedVersions()

	availableComponents := c.FilterStatus(DBEngineComponentAvailable)
	availableVersions := availableComponents.GetSortedVersions()

	return append(recommendedVersions, availableVersions...)
}

// BestVersion returns the best version for the components map.
// In case no versions are found, it returns an empty string.
func (c ComponentsMap) BestVersion() string {
	allowedVersions := c.GetAllowedVersionsSorted()
	if len(allowedVersions) == 0 {
		return ""
	}

	return allowedVersions[0]
}

// BestEngineVersion returns the best engine version for the database engine.
func (d DatabaseEngine) BestEngineVersion() string {
	return d.Status.AvailableVersions.Engine.BestVersion()
}

// BestBackupVersion returns the best backup version for a given engine version.
func (d DatabaseEngine) BestBackupVersion(engineVersion string) string {
	switch d.Spec.Type {
	case DatabaseEnginePXC:
		engineGoVersion, err := goversion.NewVersion(engineVersion)
		if err != nil {
			return ""
		}

		v8, err := goversion.NewVersion("8.0.0")
		if err != nil {
			return ""
		}

		engineIsV8 := engineGoVersion.GreaterThanOrEqual(v8)
		allowedVersions := d.Status.AvailableVersions.Backup.GetAllowedVersionsSorted()
		for _, version := range allowedVersions {
			v, err := goversion.NewVersion(version)
			if err != nil {
				continue
			}
			if !engineIsV8 && v.GreaterThanOrEqual(v8) {
				continue
			}
			if engineIsV8 && v.LessThan(v8) {
				continue
			}
			return version
		}
		return ""
	case DatabaseEnginePSMDB:
		return d.Status.AvailableVersions.Backup.BestVersion()
	case DatabaseEnginePostgresql:
		if d.Status.AvailableVersions.Backup[engineVersion] == nil {
			return ""
		}
		return engineVersion
	default:
		return ""
	}
}

func init() {
	SchemeBuilder.Register(&DatabaseEngine{}, &DatabaseEngineList{})
}
