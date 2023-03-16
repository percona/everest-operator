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
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	goversion "github.com/hashicorp/go-version"
	"github.com/pkg/errors"
)

const (
	defaultVersionServiceURL        = "https://check.percona.com/versions/v1/psmdb-operator/%s"
	versionServiceStatusRecommended = "recommended"
	psmdbBackupImageTmpl            = "percona/percona-server-mongodb-operator:%s-backup"
)

type (
	// Version is a wrapper around github.com/hashicorp/go-version that adds additional
	// functions for developer's usability.
	Version struct {
		version *goversion.Version
	}
	// Image is contains needed fields to parse information from version service.
	Image struct {
		ImagePath string `json:"imagePath"`
		ImageHash string `json:"imageHash"`
		Status    string `json:"status"`
	}
	// VersionResponse is a response model for version service response parsing.
	VersionResponse struct {
		Versions []struct {
			Matrix struct {
				Backup map[string]Image `json:"backup"`
			} `json:"matrix"`
		} `json:"versions"`
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

// PSMDBBackupImage returns backup image for psmdb clusters depending on operator version
// For 1.12+ it gets image from version service.
func (v *Version) PSMDBBackupImage() (string, error) {
	ver, _ := goversion.NewVersion("v1.11.0")
	if v.version.GreaterThan(ver) {
		resp, err := http.Get(fmt.Sprintf(defaultVersionServiceURL, v.ToCRVersion())) //nolint:noctx
		if err != nil {
			return "", err
		}
		defer resp.Body.Close() //nolint:errcheck,gosec
		var vr VersionResponse
		if err := json.NewDecoder(resp.Body).Decode(&vr); err != nil {
			return "", err
		}
		if len(vr.Versions) == 0 {
			return "", errors.New("no versions returned from version service")
		}

		for _, image := range vr.Versions[0].Matrix.Backup {
			if image.Status == versionServiceStatusRecommended {
				return image.ImagePath, nil
			}
		}
		return "", errors.New("no versions available for backup image")
	}
	return fmt.Sprintf(psmdbBackupImageTmpl, v.ToCRVersion()), nil
}
