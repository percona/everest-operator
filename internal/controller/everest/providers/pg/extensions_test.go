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

package pg

import (
	"testing"

	"github.com/AlekSi/pointer"
	pgv2 "github.com/percona/percona-postgresql-operator/v3/pkg/apis/pgv2.percona.com/v2"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"

	everestv1alpha1 "github.com/percona/everest-operator/api/everest/v1alpha1"
)

func TestApplyExtensions(t *testing.T) {
	t.Parallel()

	const image = "percona/percona-postgresql-operator:3.1.0"
	base := func() pgv2.ExtensionsSpec {
		return pgv2.ExtensionsSpec{Image: image, ImagePullPolicy: corev1.PullIfNotPresent}
	}
	on := &everestv1alpha1.PostgreSQLExtension{Enabled: pointer.ToBool(true)}
	off := &everestv1alpha1.PostgreSQLExtension{Enabled: pointer.ToBool(false)}

	testCases := []struct {
		name       string
		extensions *everestv1alpha1.PostgreSQLExtensions
		want       pgv2.BuiltInExtensionsSpec
	}{
		{
			name: "no extensions block keeps operator defaults",
		},
		{
			name:       "empty extensions block keeps operator defaults",
			extensions: &everestv1alpha1.PostgreSQLExtensions{},
		},
		{
			name: "declared extensions are mapped",
			extensions: &everestv1alpha1.PostgreSQLExtensions{
				PGVector:         on,
				PGStatStatements: on,
				PGStatMonitor:    off,
				PGAudit:          off,
				PGRepack:         on,
			},
			want: pgv2.BuiltInExtensionsSpec{
				PGVector:         pointer.ToBool(true),
				PGStatStatements: pointer.ToBool(true),
				PGStatMonitor:    pointer.ToBool(false),
				PGAudit:          pointer.ToBool(false),
				PGRepack:         pointer.ToBool(true),
			},
		},
		{
			name: "extension without enabled keeps the operator default",
			extensions: &everestv1alpha1.PostgreSQLExtensions{
				PGVector: on,
				PGAudit:  &everestv1alpha1.PostgreSQLExtension{},
			},
			want: pgv2.BuiltInExtensionsSpec{PGVector: pointer.ToBool(true)},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			spec := base()
			applyExtensions(&spec, tc.extensions)

			want := base()
			want.BuiltIn = tc.want
			assert.Equal(t, want, spec)
		})
	}

	t.Run("does not alias the DatabaseCluster spec", func(t *testing.T) {
		t.Parallel()

		extensions := &everestv1alpha1.PostgreSQLExtensions{PGVector: &everestv1alpha1.PostgreSQLExtension{Enabled: pointer.ToBool(true)}}
		spec := base()
		applyExtensions(&spec, extensions)
		*spec.BuiltIn.PGVector = false

		assert.True(t, *extensions.PGVector.Enabled)
	})
}
