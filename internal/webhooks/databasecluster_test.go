package webhooks

import (
	"testing"

	"github.com/percona/everest-operator/api/v1alpha1"
	"github.com/stretchr/testify/require"
	"gotest.tools/assert"
)

func TestCheckJSONKeyExists(t *testing.T) {
	testCases := []struct {
		obj      any
		key      string
		expected bool
	}{
		{
			obj: v1alpha1.DatabaseCluster{
				Spec: v1alpha1.DatabaseClusterSpec{
					Engine: v1alpha1.Engine{},
				},
			},
			key: ".spec.engine.type",
		},
		{
			obj: v1alpha1.DatabaseCluster{
				Spec: v1alpha1.DatabaseClusterSpec{
					Engine: v1alpha1.Engine{
						Type: v1alpha1.DatabaseEnginePSMDB,
					},
				},
			},
			key:      ".spec.engine.type",
			expected: true,
		},
		{
			obj: v1alpha1.DatabaseCluster{
				Spec: v1alpha1.DatabaseClusterSpec{
					Engine: v1alpha1.Engine{
						UserSecretsName: "my-user-secrets",
					},
				},
			},
			key:      ".spec.engine.userSecretsName",
			expected: true,
		},
		{
			obj: v1alpha1.DatabaseCluster{
				Spec: v1alpha1.DatabaseClusterSpec{
					Engine: v1alpha1.Engine{},
				},
			},
			key: ".spec.engine.userSecretsName",
		},
	}
	for _, tc := range testCases {
		exists, err := checkJSONKeyExists(tc.key, tc.obj)
		require.NoError(t, err)
		assert.Equal(t, exists, tc.expected, "Key existence check failed for key: %s", tc.key)
	}
}
