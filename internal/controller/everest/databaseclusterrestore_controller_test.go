// everest
// Copyright (C) 2025 Percona LLC
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
package everest

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	everestv1alpha1 "github.com/percona/everest-operator/api/everest/v1alpha1"
)

func TestGetPGRestoreOptions(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		data    everestv1alpha1.DatabaseClusterRestoreDataSource
		options []string
	}{
		{
			name:    "only backup",
			data:    everestv1alpha1.DatabaseClusterRestoreDataSource{},
			options: []string{"--set=smth", "--type=immediate"},
		},
		{
			name: "pitr with date",
			data: everestv1alpha1.DatabaseClusterRestoreDataSource{
				PITR: &everestv1alpha1.PITR{
					Date: &everestv1alpha1.RestoreDate{},
				},
			},
			options: []string{"--set=smth", "--type=time", "--target=\"0001-01-01 00:00:00\""},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			res, err := getPGRestoreOptions(tc.data, "smth")
			require.NoError(t, err)
			require.Equal(t, tc.options, res)
		})
	}
}

func Test_parsePrefixFromDestination(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"s3://bucketname/dbname/db-uid/backupname": "dbname/db-uid",
		"s3://percona-test-backup-storage-2/mongodb-jxs/b6968af3-dbf4-431f-a8a8-630835081abd/2024-01-10T10:56:13Z": "mongodb-jxs/b6968af3-dbf4-431f-a8a8-630835081abd",
	}
	for source, expected := range cases {
		assert.Equal(t, expected, parsePrefixFromDestination(source))
	}
}
