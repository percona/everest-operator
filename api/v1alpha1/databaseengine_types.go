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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// DBEngineStateNotInstalled represents the state of engine when underlying operator is not installed.
	DBEngineStateNotInstalled EngineState = "not installed"
	// DBEngineStateInstalling represents the state of engine when underlying operator is installing.
	DBEngineStateInstalling EngineState = "installing"
	// DBEngineStateInstalled represents the state of engine when underlying operator is installed.
	DBEngineStateInstalled EngineState = "installed"
	// DatabaseEnginePXC represents engine type for PXC clusters.
	DatabaseEnginePXC EngineType = "pxc"
	// DatabaseEnginePSMDB represents engine type for PSMDB clusters.
	DatabaseEnginePSMDB EngineType = "psmdb"
	// DatabaseEnginePostgresql represents engine type for Postgresql clusters.
	DatabaseEnginePostgresql EngineType = "postgresql"
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
	Type            EngineType `json:"type"`
	AllowedVersions []string   `json:"allowedVersions,omitempty"`
}

// DatabaseEngineStatus defines the observed state of DatabaseEngine.
type DatabaseEngineStatus struct {
	State             EngineState `json:"status,omitempty"`
	OperatorVersion   string      `json:"operatorVersion,omitempty"`
	AvailableVersions Versions    `json:"availableVersions,omitempty"`
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

// Component contains information of the database engine component.
// Database Engine component can be database engine, database proxy or tools image path.
type Component struct {
	Critical  bool   `json:"critical,omitempty"`
	ImageHash string `json:"imageHash,omitempty"`
	ImagePath string `json:"imagePath,omitempty"`
	Status    string `json:"status,omitempty"`
}

// RecommendedVersion returns the recommended version for the components map.
func (c ComponentsMap) RecommendedVersion() string {
	var versions []*goversion.Version //nolint:prealloc
	for version, component := range c {
		if component.Status != "recommended" {
			continue
		}
		v, err := goversion.NewVersion(version)
		if err != nil {
			continue
		}
		versions = append(versions, v)
	}
	if len(versions) == 0 {
		return ""
	}
	sort.Sort(goversion.Collection(versions))

	return versions[len(versions)-1].Original()
}

// RecommendedEngineVersion returns the recommended version for the database engine.
func (d DatabaseEngine) RecommendedEngineVersion() string {
	return d.Status.AvailableVersions.Engine.RecommendedVersion()
}

// RecommendedBackupImage returns the recommended image for the backup component.
func (d DatabaseEngine) RecommendedBackupImage() string {
	for _, component := range d.Status.AvailableVersions.Backup {
		if component.Status == "recommended" {
			return component.ImagePath
		}
	}
	return ""
}

func init() {
	SchemeBuilder.Register(&DatabaseEngine{}, &DatabaseEngineList{})
}
