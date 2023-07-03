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

package controllers

import (
	"fmt"
	"strings"

	goversion "github.com/hashicorp/go-version"
)

type (
	// Version is a wrapper around github.com/hashicorp/go-version that adds additional
	// functions for developer's usability.
	Version struct {
		version *goversion.Version
	}
)

// NewVersion creates a new version from given string.
func NewVersion(v string) (*Version, error) {
	version, err := goversion.NewVersion(v)
	if err != nil {
		return nil, err
	}
	return &Version{version: version}, nil
}

// String returns version string.
func (v *Version) String() string {
	return v.version.String()
}

// ToCRVersion returns version usable as CRversion parameter.
func (v *Version) ToCRVersion() string {
	return strings.ReplaceAll(v.String(), "v", "")
}

// ToSemver returns version is semver format.
func (v *Version) ToSemver() string {
	return fmt.Sprintf("v%s", v.String())
}

// ToAPIVersion returns version that can be used as K8s APIVersion parameter.
func (v *Version) ToAPIVersion(apiRoot string) string {
	ver, _ := goversion.NewVersion("v1.12.0")
	if v.version.GreaterThan(ver) {
		return fmt.Sprintf("%s/v1", apiRoot)
	}
	return fmt.Sprintf("%s/v%s", apiRoot, strings.ReplaceAll(v.String(), ".", "-"))
}
