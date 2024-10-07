// everest-operator Copyright (C) 2022 Percona LLC
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

package predicates

import (
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func newNamespace(labels map[string]string) *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   "test-ns",
			Labels: labels,
		},
	}
}

func TestNamespaceFilter(t *testing.T) {
	t.Parallel()
	testCases := []struct {
		namespace *corev1.Namespace
		filter    *NamespaceFilter
		expected  bool
	}{
		{
			namespace: newNamespace(nil),
			filter:    &NamespaceFilter{Enabled: false},
			expected:  true,
		},
		{
			namespace: newNamespace(nil),
			filter:    &NamespaceFilter{Enabled: true},
			expected:  true,
		},
		{
			namespace: newNamespace(nil),
			filter: &NamespaceFilter{
				Enabled:         true,
				AllowNamespaces: []string{"test"},
			},
			expected: true,
		},
		{
			namespace: newNamespace(nil),
			filter: &NamespaceFilter{
				Enabled:     true,
				MatchLabels: map[string]string{"key": "value"},
			},
			expected: false,
		},
		{
			namespace: newNamespace(map[string]string{"key": "value"}),
			filter: &NamespaceFilter{
				Enabled:     true,
				MatchLabels: map[string]string{"key": "value"},
			},
			expected: true,
		},
	}

	for i, tc := range testCases {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()
			actual := tc.filter.filterNamespace(tc.namespace)
			assert.Equal(t, tc.expected, actual)
		})
	}
}
