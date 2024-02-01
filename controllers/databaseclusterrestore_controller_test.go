package controllers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	everestv1alpha1 "github.com/percona/everest-operator/api/v1alpha1"
)

func TestValidatePitrRestoreSpec(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		data everestv1alpha1.DataSource
		err  error
	}{
		{
			name: "empty date",
			data: everestv1alpha1.DataSource{
				PITR: &everestv1alpha1.PITR{},
			},
			err: errPitrEmptyDate,
		},
		{
			name: "type latest",
			data: everestv1alpha1.DataSource{
				PITR: &everestv1alpha1.PITR{
					Type: "latest",
				},
			},
			err: errPitrTypeLatest,
		},
		{
			name: "type unknown",
			data: everestv1alpha1.DataSource{
				PITR: &everestv1alpha1.PITR{
					Type: "some-type",
				},
			},
			err: errPitrTypeIsNotSupported,
		},
		{
			name: "no error with type",
			data: everestv1alpha1.DataSource{
				PITR: &everestv1alpha1.PITR{
					Type: "date",
					Date: &everestv1alpha1.RestoreDate{},
				},
			},
			err: nil,
		},
		{
			name: "no error without type",
			data: everestv1alpha1.DataSource{
				PITR: &everestv1alpha1.PITR{
					Date: &everestv1alpha1.RestoreDate{},
				},
			},
			err: nil,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := validatePitrRestoreSpec(tc.data)
			if tc.err == nil {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.Equal(t, err.Error(), tc.err.Error())
		})
	}
}

func TestGetPGRestoreOptions(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		data    everestv1alpha1.DatabaseClusterRestore
		options []string
	}{
		{
			name:    "only backup",
			data:    everestv1alpha1.DatabaseClusterRestore{},
			options: []string{"--set=smth", "--type=immediate"},
		},
		{
			name: "pitr with date",
			data: everestv1alpha1.DatabaseClusterRestore{
				Spec: everestv1alpha1.DatabaseClusterRestoreSpec{
					DataSource: everestv1alpha1.DataSource{
						PITR: &everestv1alpha1.PITR{
							Date: &everestv1alpha1.RestoreDate{},
						},
					},
				},
			},
			options: []string{"--set=smth", "--type=time", "--target=\"0001-01-01 00:00:00\""},
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			res, err := getPGRestoreOptions(&tc.data, "smth")
			require.NoError(t, err)
			require.Equal(t, tc.options, res)
		})
	}
}
