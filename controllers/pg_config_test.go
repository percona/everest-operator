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

package controllers

import (
	"reflect"
	"testing"
)

func TestPGConfigParser_ParsePGConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		config  string
		want    map[string]any
		wantErr bool
	}{
		{
			name:   "parse one value",
			config: "name = value",
			want: map[string]any{
				"name": "value",
			},
			wantErr: false,
		},
		{
			name:   "parse one value with new line",
			config: "name = value\n",
			want: map[string]any{
				"name": "value",
			},
			wantErr: false,
		},
		{
			name: "parse many values",
			config: `
			name1 = value1
			name2 = value2
			name3 = value3
			`,
			want: map[string]any{
				"name1": "value1",
				"name2": "value2",
				"name3": "value3",
			},
			wantErr: false,
		},
		{
			name: "parse many values mixed spaces and equal signs",
			config: `
			name1 = value1
			name2 value2
			name3 = value3
			`,
			want: map[string]any{
				"name1": "value1",
				"name2": "value2",
				"name3": "value3",
			},
			wantErr: false,
		},
		{
			name: "parse complex config",
			config: `
			name1 = value1
			name2 value2
			# comment
			name3 = value3 # comment

			# comment
			name4 value4
			# name5 value5

			`,
			want: map[string]any{
				"name1": "value1",
				"name2": "value2",
				"name3": "value3",
				"name4": "value4",
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := &PGConfigParser{
				config: tt.config,
			}
			got, err := p.ParsePGConfig()
			if err != nil {
				t.Errorf("PGConfigParser.ParsePGConfig() error = %v wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("PGConfigParser.parsePGConfig() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPGConfigParser_lineUsesEqualSign(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		line     string
		useEqual bool
	}{
		{
			name:     "equal - standard",
			line:     "name = value",
			useEqual: true,
		},
		{
			name:     "equal - no spaces",
			line:     "name=value",
			useEqual: true,
		},
		{
			name:     "equal - no spaces but space in value",
			line:     "name=value abc",
			useEqual: true,
		},
		{
			name:     "equal - no spaces before",
			line:     "name= value",
			useEqual: true,
		},
		{
			name:     "equal - no spaces after",
			line:     "name =value",
			useEqual: true,
		},
		{
			name:     "equal - many spaces",
			line:     "name   =    value",
			useEqual: true,
		},
		{
			name:     "equal - many spaces before",
			line:     "name   = value",
			useEqual: true,
		},
		{
			name:     "equal - many spaces after",
			line:     "name =   value",
			useEqual: true,
		},
		{
			name:     "no equal - standard",
			line:     "name value",
			useEqual: false,
		},
		{
			name:     "no equal - more spaces",
			line:     "name  value",
			useEqual: false,
		},
		{
			name:     "no equal - equal in value with space",
			line:     "name value =",
			useEqual: false,
		},
		{
			name:     "no equal - equal in value with no space",
			line:     "name value=",
			useEqual: false,
		},
		{
			name:     "no equal - multiple spaces, equal in value with no space",
			line:     "name  value=",
			useEqual: false,
		},
		{
			name:     "no equal - multiple spaces, equal in value with space",
			line:     "name  value =",
			useEqual: false,
		},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := &PGConfigParser{config: ""}
			got := p.lineUsesEqualSign([]byte(tt.line))
			if tt.useEqual != got {
				t.Errorf("Did not detect equal sign properly for test %s %q", tt.name, tt.line)
			}
		})
	}
}
