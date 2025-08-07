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
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/percona/everest-operator/api/v1alpha1"
)

func TestGetPMMResources(t *testing.T) {
	t.Parallel()

	type res struct {
		memory string
		cpu    string
	}

	type resourceString struct {
		requests res
		limits   res
	}

	tests := []struct {
		name           string
		monitoringSpec v1alpha1.Monitoring
		dbEnginSize    v1alpha1.EngineSize
		want           corev1.ResourceRequirements
		wantString     resourceString
	}{
		{
			name:           "PMM resources for small cluster",
			monitoringSpec: v1alpha1.Monitoring{},
			dbEnginSize:    v1alpha1.EngineSizeSmall,
			want:           pmmResourceRequirementsSmall,
			wantString: resourceString{
				requests: res{
					memory: "99604Ki",
					cpu:    "95m",
				},
			},
		},
		{
			name:           "PMM resources for medium cluster",
			monitoringSpec: v1alpha1.Monitoring{},
			dbEnginSize:    v1alpha1.EngineSizeMedium,
			want:           pmmResourceRequirementsMedium,
			wantString: resourceString{
				requests: res{
					memory: "199168Ki",
					cpu:    "228m",
				},
			},
		},
		{
			name:           "PMM resources for large cluster",
			monitoringSpec: v1alpha1.Monitoring{},
			dbEnginSize:    v1alpha1.EngineSizeLarge,
			want:           pmmResourceRequirementsLarge,
			wantString: resourceString{
				requests: res{
					memory: "796907Ki",
					cpu:    "228m",
				},
			},
		},
		{
			name: "PMM custom resources(requests) for cluster",
			monitoringSpec: v1alpha1.Monitoring{
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceMemory: *resource.NewQuantity(97*1024*1024, resource.BinarySI),
						corev1.ResourceCPU:    *resource.NewScaledQuantity(95, resource.Milli),
					},
				},
			},
			dbEnginSize: v1alpha1.EngineSizeSmall,
			want: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceMemory: *resource.NewQuantity(97*1024*1024, resource.BinarySI),
					corev1.ResourceCPU:    *resource.NewScaledQuantity(95, resource.Milli),
				},
			},
			wantString: resourceString{
				requests: res{
					memory: "97Mi",
					cpu:    "95m",
				},
			},
		},
		{
			name: "PMM custom resources(limits) for cluster",
			monitoringSpec: v1alpha1.Monitoring{
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceMemory: *resource.NewQuantity(197*1024*1024, resource.BinarySI),
						corev1.ResourceCPU:    *resource.NewScaledQuantity(195, resource.Milli),
					},
				},
			},
			dbEnginSize: v1alpha1.EngineSizeSmall,
			want: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceMemory: *resource.NewQuantity(97*1024*1024+276*1024, resource.BinarySI),
					corev1.ResourceCPU:    *resource.NewScaledQuantity(95, resource.Milli),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: *resource.NewQuantity(197*1024*1024, resource.BinarySI),
					corev1.ResourceCPU:    *resource.NewScaledQuantity(195, resource.Milli),
				},
			},
			wantString: resourceString{
				requests: res{
					memory: "99604Ki",
					cpu:    "95m",
				},
				limits: res{
					memory: "197Mi",
					cpu:    "195m",
				},
			},
		},
		{
			name: "PMM custom resources(requests+limits) for cluster",
			monitoringSpec: v1alpha1.Monitoring{
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceMemory: *resource.NewQuantity(197*1024*1024, resource.BinarySI),
						corev1.ResourceCPU:    *resource.NewScaledQuantity(195, resource.Milli),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceMemory: *resource.NewQuantity(297*1024*1024, resource.BinarySI),
						corev1.ResourceCPU:    *resource.NewScaledQuantity(295, resource.Milli),
					},
				},
			},
			dbEnginSize: v1alpha1.EngineSizeSmall,
			want: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceMemory: *resource.NewQuantity(197*1024*1024, resource.BinarySI),
					corev1.ResourceCPU:    *resource.NewScaledQuantity(195, resource.Milli),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: *resource.NewQuantity(297*1024*1024, resource.BinarySI),
					corev1.ResourceCPU:    *resource.NewScaledQuantity(295, resource.Milli),
				},
			},
			wantString: resourceString{
				requests: res{
					memory: "197Mi",
					cpu:    "195m",
				},
				limits: res{
					memory: "297Mi",
					cpu:    "295m",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			calculatedResources := GetPMMResources(tt.monitoringSpec, tt.dbEnginSize)
			assert.Equal(t, tt.want, calculatedResources)

			if tt.wantString.requests.memory != "" {
				assert.Equal(t, tt.wantString.requests.memory, calculatedResources.Requests.Memory().String())
			}

			if tt.wantString.requests.cpu != "" {
				assert.Equal(t, tt.wantString.requests.cpu, calculatedResources.Requests.Cpu().String())
			}

			if tt.wantString.limits.memory != "" {
				assert.Equal(t, tt.wantString.limits.memory, calculatedResources.Limits.Memory().String())
			}

			if tt.wantString.limits.cpu != "" {
				assert.Equal(t, tt.wantString.limits.cpu, calculatedResources.Limits.Cpu().String())
			}
		})
	}
}
