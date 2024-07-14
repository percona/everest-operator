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
