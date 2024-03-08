// everest-operator
// Copyright (C) 2024 Percona LLC
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

import "strconv"

const (
	// haproxyConfigDefault is the default HAProxy configuration.
	haProxyConfigDefault = `
global
    maxconn 4048
defaults
    timeout connect 100500ms
    timeout client 28800ms
    timeout server 28800ms
    `
)

var (
	// haProxyEnvVars contains the environment variables to be set in the HAProxy container.
	haProxyEnvVars = map[string][]byte{
		"HA_CONNECTION_TIMEOUT": []byte(strconv.Itoa(5000)),
	}
)
