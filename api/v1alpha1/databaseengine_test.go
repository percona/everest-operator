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
package v1alpha1

import "testing"

func TestGetNextUpgradeVersion(t *testing.T) {
	testCases := []struct {
		status *DatabaseEngineStatus
		want   string
	}{
		{
			status: &DatabaseEngineStatus{
				PendingOperatorUpgrades: []OperatorUpgrade{},
			},
			want: "",
		},
		{
			status: &DatabaseEngineStatus{
				PendingOperatorUpgrades: []OperatorUpgrade{
					{TargetVersion: "1.0.0"},
				},
			},
			want: "1.0.0",
		},
		{
			status: &DatabaseEngineStatus{
				PendingOperatorUpgrades: []OperatorUpgrade{
					{TargetVersion: "1.0.0"},
					{TargetVersion: "1.1.0"},
				},
			},
			want: "1.0.0",
		},
	}

	for _, tc := range testCases {
		got := tc.status.GetNextUpgradeVersion()
		if got != tc.want {
			t.Errorf("got %s, want %s", got, tc.want)
		}
	}
}
