package v1alpha1

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestValidateSchema(t *testing.T) {
	tests := []struct {
		name       string
		schemaJSON string
		paramsJSON string
		expectErr  bool
	}{
		{
			name: "Valid parameters",
			schemaJSON: `{
				"type": "object",
				"properties": {
					"key": {"type": "string"}
				},
				"required": ["key"]
			}`,
			paramsJSON: `{"key": "value"}`,
			expectErr:  false,
		},
		{
			name: "Missing required field",
			schemaJSON: `{
				"type": "object",
				"properties": {
					"key": {"type": "string"}
				},
				"required": ["key"]
			}`,
			paramsJSON: `{}`,
			expectErr:  true,
		},
		{
			name: "Invalid type",
			schemaJSON: `{
				"type": "object",
				"properties": {
					"key": {"type": "string"}
				}
			}`,
			paramsJSON: `{"key": 123}`,
			expectErr:  true,
		},
		{
			name: "Object with multiple keys and nested objects",
			schemaJSON: `{
				"type": "object",
				"properties": {
					"key1": {"type": "string"},
					"key2": {"type": "integer"},
					"nested": {
						"type": "object",
						"properties": {
							"subkey1": {"type": "boolean"},
							"subkey2": {"type": "array", "items": {"type": "string"}}
						},
						"required": ["subkey1"]
					}
				},
				"required": ["key1", "nested"]
			}`,
			paramsJSON: `{
				"key1": "value1",
				"key2": 42,
				"nested": {
					"subkey1": true,
					"subkey2": ["item1", "item2"]
				}
			}`,
			expectErr: false,
		},
		{
			name: "Object with missing nested required field",
			schemaJSON: `{
				"type": "object",
				"properties": {
					"key1": {"type": "string"},
					"key2": {"type": "integer"},
					"nested": {
						"type": "object",
						"properties": {
							"subkey1": {"type": "boolean"},
							"subkey2": {"type": "array", "items": {"type": "string"}}
						},
						"required": ["subkey1"]
					}
				},
				"required": ["key1", "nested"]
			}`,
			paramsJSON: `{
				"key1": "value1",
				"key2": 42,
				"nested": {
					"subkey2": ["item1", "item2"]
				}
			}`,
			expectErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var schema apiextensionsv1.JSONSchemaProps
			var params runtime.RawExtension

			// Unmarshal inputs
			assert.NoError(t, json.Unmarshal([]byte(tt.schemaJSON), &schema))
			params.Raw = []byte(tt.paramsJSON)

			cfg := DataImporterConfig{
				OpenAPIV3Schema: &schema,
			}

			err := cfg.ValidateParams(&params)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
