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
	"os"

	"github.com/pkg/errors"

	dbaasv1 "github.com/percona/dbaas-operator/api/v1"
)

type (
	// VersionService used for the integration with Percona Version Service.
	VersionService struct {
		url string
	}
	// Matrix represents the response from the version service.
	Matrix struct {
		Backup       map[string]*dbaasv1.Component `json:"backup"`
		Mongod       map[string]*dbaasv1.Component `json:"mongod"`
		PXC          map[string]*dbaasv1.Component `json:"pxc"`
		ProxySQL     map[string]*dbaasv1.Component `json:"proxysql"`
		HAProxy      map[string]*dbaasv1.Component `json:"haproxy"`
		LogCollector map[string]*dbaasv1.Component `json:"logCollector"`
		Postgresql   map[string]*dbaasv1.Component `json:"postgresql"`
		PGBackRest   map[string]*dbaasv1.Component `json:"pgbackrest"`
		PGBouncer    map[string]*dbaasv1.Component `json:"pgbouncer"`
	}
	// VersionResponse is a response model for version service response parsing.
	VersionResponse struct {
		Versions []struct {
			Matrix Matrix `json:"matrix"`
		} `json:"versions"`
	}
)

const (
	defaultVersionServiceURL        = "https://check.percona.com/versions/v1"
	versionServiceStatusRecommended = "recommended"
)

var operatorNames = map[dbaasv1.EngineType]string{
	dbaasv1.DatabaseEnginePXC:        "pxc-operator",
	dbaasv1.DatabaseEnginePSMDB:      "psmdb-operator",
	dbaasv1.DatabaseEnginePostgresql: "pg-operator",
}

// NewVersionService creates a version service client.
func NewVersionService() *VersionService {
	versionServiceURL := os.Getenv("PERCONA_VERSION_SERVICE_URL")
	if versionServiceURL == "" {
		versionServiceURL = defaultVersionServiceURL
	}
	return &VersionService{url: versionServiceURL}
}

// GetVersions returns a matrix of available versions for a database engine.
func (v *VersionService) GetVersions(engineType dbaasv1.EngineType, operatorVersion string) (*Matrix, error) {
	resp, err := http.Get(fmt.Sprintf("%s/%s/%s", v.url, operatorNames[engineType], operatorVersion)) //nolint:noctx
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close() //nolint:errcheck
	var vr VersionResponse
	if err := json.NewDecoder(resp.Body).Decode(&vr); err != nil {
		return nil, err
	}
	if len(vr.Versions) == 0 {
		return nil, errors.New("no versions returned from version service")
	}
	return &vr.Versions[0].Matrix, nil
}
