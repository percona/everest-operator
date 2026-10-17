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
	crunchyv1beta1 "github.com/percona/percona-postgresql-operator/v3/pkg/apis/upstream.pgv2.percona.com/v1beta1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/util/json"
)

func TestApplySingleInstancePatroniDCSTolerance(t *testing.T) {
	t.Parallel()

	pgParams := func() crunchyv1beta1.SchemalessObject {
		return crunchyv1beta1.SchemalessObject{
			"postgresql": map[string]any{
				"parameters": map[string]any{"track_commit_timestamp": "on"},
			},
		}
	}

	testCases := []struct {
		name     string
		replicas int32
		patroni  *crunchyv1beta1.PatroniSpec
		want     *crunchyv1beta1.PatroniSpec
	}{
		{
			name:     "single instance without patroni spec",
			replicas: 1,
			want: &crunchyv1beta1.PatroniSpec{
				LeaderLeaseDurationSeconds: pointer.ToInt32(300),
				DynamicConfiguration: crunchyv1beta1.SchemalessObject{
					"failsafe_mode": true,
					"retry_timeout": int64(120),
				},
			},
		},
		{
			name:     "single instance keeps engine config parameters",
			replicas: 1,
			patroni:  &crunchyv1beta1.PatroniSpec{DynamicConfiguration: pgParams()},
			want: &crunchyv1beta1.PatroniSpec{
				LeaderLeaseDurationSeconds: pointer.ToInt32(300),
				DynamicConfiguration: func() crunchyv1beta1.SchemalessObject {
					dc := pgParams()
					dc["failsafe_mode"] = true
					dc["retry_timeout"] = int64(120)
					return dc
				}(),
			},
		},
		{
			name:     "multiple instances keep patroni defaults",
			replicas: 3,
			patroni:  &crunchyv1beta1.PatroniSpec{DynamicConfiguration: pgParams()},
			want:     &crunchyv1beta1.PatroniSpec{DynamicConfiguration: pgParams()},
		},
		{
			name:     "multiple instances without patroni spec",
			replicas: 3,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			pg := &pgv2.PerconaPGCluster{}
			pg.Spec.Patroni = tc.patroni
			applySingleInstancePatroniDCSTolerance(pg, tc.replicas)
			assert.Equal(t, tc.want, pg.Spec.Patroni)

			// The spec is deep-copied by the client and compared against the
			// object read back from the API server on every reconcile, so the
			// values must survive both without a panic or a spurious diff.
			require.NotPanics(t, func() { pg.DeepCopy() })
			raw, err := json.Marshal(pg.Spec)
			require.NoError(t, err)
			var fromAPI pgv2.PerconaPGClusterSpec
			require.NoError(t, json.Unmarshal(raw, &fromAPI))
			assert.True(t, equality.Semantic.DeepEqual(pg.Spec, fromAPI))
		})
	}
}
