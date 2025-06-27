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

	"github.com/stretchr/testify/assert"
)

func TestParseBackupPath(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name            string
		backupPath      string
		wantBackupName  string
		wantDBDirectory string
	}{
		{
			name:            "standard path",
			backupPath:      "/my-database/backup/db/backup-1",
			wantBackupName:  "backup-1",
			wantDBDirectory: "/my-database/",
		},
		{
			name:            "path with trailing slash",
			backupPath:      "/my-database/backup/db/backup-1/",
			wantBackupName:  "backup-1",
			wantDBDirectory: "/my-database/",
		},
		{
			name:            "path without leading slash",
			backupPath:      "my-database/backup/db/backup-1",
			wantBackupName:  "backup-1",
			wantDBDirectory: "/my-database/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			backupName, dbDir := parseBackupPath(tt.backupPath)
			assert.Equal(t, tt.wantBackupName, backupName)
			assert.Equal(t, tt.wantDBDirectory, dbDir)
		})
	}
}
