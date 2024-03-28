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

package pxc

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

var ( //nolint:dupl
	// A pmmResourceRequirementsSmall is the resource requirements for PMM for small clusters.
	pmmResourceRequirementsSmall = corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceMemory: resource.MustParse("97.27Mi"),
			corev1.ResourceCPU:    resource.MustParse("95m"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceMemory: resource.MustParse("102.4Mi"),
			corev1.ResourceCPU:    resource.MustParse("100m"),
		},
	}

	// A pmmResourceRequirementsMedium is the resource requirements for PMM for medium clusters.
	pmmResourceRequirementsMedium = corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceMemory: resource.MustParse("194.5Mi"),
			corev1.ResourceCPU:    resource.MustParse("228m"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceMemory: resource.MustParse("204.7Mi"),
			corev1.ResourceCPU:    resource.MustParse("240m"),
		},
	}

	// A pmmResourceRequirementsLarge is the resource requirements for PMM for large clusters.
	pmmResourceRequirementsLarge = corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceMemory: resource.MustParse("778.23Mi"),
			corev1.ResourceCPU:    resource.MustParse("228m"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceMemory: resource.MustParse("819.19Mi"),
			corev1.ResourceCPU:    resource.MustParse("240m"),
		},
	}
)
