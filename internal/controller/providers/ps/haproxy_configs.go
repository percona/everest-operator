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

package ps

import (
	"strconv"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	// A haproxyConfigDefault is the default HAProxy configuration.
	//nolint:lll
	haProxyConfigDefault = `
        global
          maxconn 2048
          external-check
          insecure-fork-wanted
          stats socket /etc/haproxy/mysql/haproxy.sock mode 600 expose-fd listeners level admin

        defaults
          default-server init-addr last,libc,none
          log global
          mode tcp
          retries 10
          timeout client 28800s
          timeout connect 100500
          timeout server 28800s

        frontend mysql-primary-in
          bind *:3309 accept-proxy
          bind *:3306
          mode tcp
          option clitcpka
          default_backend mysql-primary

        frontend mysql-replicas-in
          bind *:3307
          mode tcp
          option clitcpka
          default_backend mysql-replicas

        frontend stats
          bind *:8404
          mode http
          http-request use-service prometheus-exporter if { path /metrics }
`

	haConnectionTimeout = 1000
)

// A haProxyEnvVars contains the environment variables to be set in the HAProxy container.
var haProxyEnvVars = map[string][]byte{
	"HA_CONNECTION_TIMEOUT": []byte(strconv.Itoa(haConnectionTimeout)),
}

var (
	// A haProxyResourceRequirementsSmall is the resource requirements for HAProxy for small clusters.
	haProxyResourceRequirementsSmall = corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceMemory: resource.MustParse("195Mi"),
			corev1.ResourceCPU:    resource.MustParse("190m"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceMemory: resource.MustParse("204Mi"),
			corev1.ResourceCPU:    resource.MustParse("200m"),
		},
	}

	// A haProxyResourceRequirementsMedium is the resource requirements for HAProxy for medium clusters.
	haProxyResourceRequirementsMedium = corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceMemory: resource.MustParse("778Mi"),
			corev1.ResourceCPU:    resource.MustParse("532m"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceMemory: resource.MustParse("820Mi"),
			corev1.ResourceCPU:    resource.MustParse("560m"),
		},
	}

	// A haProxyResourceRequirementsLarge is the resource requirements for HAProxy for large clusters.
	haProxyResourceRequirementsLarge = corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceMemory: resource.MustParse("2.84Gi"),
			corev1.ResourceCPU:    resource.MustParse("818m"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceMemory: resource.MustParse("3Gi"),
			corev1.ResourceCPU:    resource.MustParse("861m"),
		},
	}
)
