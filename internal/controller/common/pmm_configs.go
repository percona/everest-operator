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

package common

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	everestv1alpha1 "github.com/percona/everest-operator/api/v1alpha1"
)

const (
	// DefaultPMMClientImage is the default image for PMM client.
	DefaultPMMClientImage = "percona/pmm-client:2"
	// pmmClientRequestCPUSmall are the default CPU requests for PMM client in small clusters.
	pmmClientRequestCPUSmall = 95
	// pmmClientRequestCPUSmall are the default CPU requests for PMM client in medium clusters.
	pmmClientRequestCPUMedium = 228
	// pmmClientRequestCPUSmall are the default CPU requests for PMM client in large clusters.
	pmmClientRequestCPULarge = 228
)

var (
	// NOTE: provided below values were taken from the tool https://github.com/Tusamarco/mysqloperatorcalculator

	// A pmmResourceRequirementsSmall is the resource requirements for PMM for small clusters.
	pmmResourceRequirementsSmall = corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			// 97.27Mi = 97 MiB + 276 KiB = 99604 KiB
			corev1.ResourceMemory: *resource.NewQuantity(97*1024*1024+276*1024, resource.BinarySI),
			corev1.ResourceCPU:    *resource.NewScaledQuantity(pmmClientRequestCPUSmall, resource.Milli),
		},
	}

	// A pmmResourceRequirementsMedium is the resource requirements for PMM for medium clusters.
	pmmResourceRequirementsMedium = corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			// 194.5Mi = 194 MiB + 512 KiB = 199168 KiB
			corev1.ResourceMemory: *resource.NewQuantity(194*1024*1024+512*1024, resource.BinarySI),
			corev1.ResourceCPU:    *resource.NewScaledQuantity(pmmClientRequestCPUMedium, resource.Milli),
		},
	}

	// A pmmResourceRequirementsLarge is the resource requirements for PMM for large clusters.
	pmmResourceRequirementsLarge = corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			// 778.23Mi = 778 MiB + 235 KiB = 796907 KiB
			corev1.ResourceMemory: *resource.NewQuantity(778*1024*1024+235*1024, resource.BinarySI),
			corev1.ResourceCPU:    *resource.NewScaledQuantity(pmmClientRequestCPULarge, resource.Milli),
		},
	}
)

// GetPMMResources returns the resource requirements for PMM based on the monitoring configuration and database engine size.
func GetPMMResources(monitoringSpec everestv1alpha1.Monitoring, dbEnginSize everestv1alpha1.EngineSize) corev1.ResourceRequirements {
	var pmmResources corev1.ResourceRequirements

	// Set PMM resources.requests from incoming monitoring config, if specified.
	if monitoringSpec.Resources.Requests != nil {
		pmmResources.Requests = monitoringSpec.Resources.Requests
	} else {
		// Set resources.requests based on cluster size.
		switch dbEnginSize {
		case everestv1alpha1.EngineSizeSmall:
			pmmResources = pmmResourceRequirementsSmall
		case everestv1alpha1.EngineSizeMedium:
			pmmResources = pmmResourceRequirementsMedium
		case everestv1alpha1.EngineSizeLarge:
			pmmResources = pmmResourceRequirementsLarge
		}
	}

	// Set PMM resources.limits from incoming monitoring config, if specified.
	if monitoringSpec.Resources.Limits != nil {
		pmmResources.Limits = monitoringSpec.Resources.Limits
	}

	return pmmResources
}
