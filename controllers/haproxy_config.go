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

package controllers

import (
	"strconv"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

const (
	// A haproxyConfigDefault is the default HAProxy configuration.
	haProxyConfigDefault = `
global
    maxconn 4048
defaults
    timeout connect 100500ms
    timeout client 28800ms
    timeout server 28800ms
    `
)

// A haProxyEnvVars contains the environment variables to be set in the HAProxy container.
var haProxyEnvVars = map[string][]byte{
	"HA_CONNECTION_TIMEOUT": []byte(strconv.Itoa(5000)),
}

var ( //nolint:dupl
	// A haProxyResourceRequirementsSmall is the resource requirements for HAProxy for small clusters.
	haProxyResourceRequirementsSmall = corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("195Mi"),
			corev1.ResourceMemory: resource.MustParse("95m"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("204Mi"),
			corev1.ResourceMemory: resource.MustParse("100m"),
		},
	}

	// A haProxyResourceRequirementsMedium is the resource requirements for HAProxy for medium clusters.
	haProxyResourceRequirementsMedium = corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("778Mi"),
			corev1.ResourceMemory: resource.MustParse("228m"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("819Mi"),
			corev1.ResourceMemory: resource.MustParse("240m"),
		},
	}

	// A haProxyResourceRequirementsLarge is the resource requirements for HAProxy for large clusters.
	haProxyResourceRequirementsLarge = corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("3.19Gi"),
			corev1.ResourceMemory: resource.MustParse("228m"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("3.04Gi"),
			corev1.ResourceMemory: resource.MustParse("240m"),
		},
	}
)
