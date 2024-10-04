package predicates

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func newNamespace(name string, labels map[string]string) *corev1.Namespace {
	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:   name,
			Labels: labels,
		},
	}
}

func TestFilterNamespace(t *testing.T) {
	testCases := []struct {
		namespace *corev1.Namespace
		filter    *namespacefilter
		expected  bool
	}{
		{
			namespace: newNamespace("test", nil),
			filter:    &namespacefilter{enabled: false},
			expected:  true,
		},
		{
			namespace: newNamespace("test", nil),
			filter:    &namespacefilter{enabled: true},
			expected:  true,
		},
		{
			namespace: newNamespace("test", nil),
			filter: &namespacefilter{
				enabled:         true,
				allowNamespaces: []string{"test"},
			},
			expected: true,
		},
		{
			namespace: newNamespace("test", nil),
			filter: &namespacefilter{
				enabled:     true,
				matchLabels: map[string]string{"key": "value"},
			},
			expected: false,
		},
		{
			namespace: newNamespace("test", map[string]string{"key": "value"}),
			filter: &namespacefilter{
				enabled:     true,
				matchLabels: map[string]string{"key": "value"},
			},
			expected: true,
		},
	}

	for i, tc := range testCases {
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			actual := tc.filter.filterNamespace(tc.namespace)
			assert.Equal(t, tc.expected, actual)
		})
	}
}
