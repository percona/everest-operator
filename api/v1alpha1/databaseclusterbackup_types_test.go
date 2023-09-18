package v1alpha1

import (
	"reflect"
	"testing"
)

func TestDatabaseClusterReconciler_toCIDR(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		ranges []IPSourceRange
		want   []string
	}{
		{
			name:   "shall not make any changes",
			ranges: []IPSourceRange{"1.1.1.1/32", "1.1.1.1/24", "2001:db8:abcd:0012::0/64", "2001:db8:abcd:0012::0/128"},
			want:   []string{"1.1.1.1/32", "1.1.1.1/24", "2001:db8:abcd:0012::0/64", "2001:db8:abcd:0012::0/128"},
		},
		{
			name:   "shall not fail with empty",
			ranges: []IPSourceRange{},
			want:   []string{},
		},
		{
			name:   "shall fix ipv4 and ipv6",
			ranges: []IPSourceRange{"1.1.1.1/32", "1.1.1.1", "2001:db8:abcd:0012::0/64", "2001:db8:abcd:0012::0"},
			want:   []string{"1.1.1.1/32", "1.1.1.1/32", "2001:db8:abcd:0012::0/64", "2001:db8:abcd:0012::0/128"},
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			e := Expose{}
			if got := e.toCIDR(tt.ranges); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Expose.parseIPSourceRanges() = %v, want %v", got, tt.want)
			}
		})
	}
}
