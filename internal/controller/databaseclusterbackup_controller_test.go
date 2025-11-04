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
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_isCRVersionGreaterOrEqual(t *testing.T) {
	t.Parallel()
	type tCase struct {
		name             string
		currentVersion   string
		desiredVersion   string
		isGreaterOrEqual bool
		err              error
	}
	cases := []tCase{
		{
			name:             "version smaller",
			currentVersion:   "1.19.0",
			desiredVersion:   "1.20.0",
			err:              nil,
			isGreaterOrEqual: false,
		},
		{
			name:             "version equal",
			currentVersion:   "1.20.0",
			desiredVersion:   "1.20.0",
			err:              nil,
			isGreaterOrEqual: true,
		},
		{
			name:             "version greater patch",
			currentVersion:   "1.20.1",
			desiredVersion:   "1.20.0",
			err:              nil,
			isGreaterOrEqual: true,
		},
		{
			name:             "version greater minor",
			currentVersion:   "1.21.0",
			desiredVersion:   "1.20.0",
			err:              nil,
			isGreaterOrEqual: true,
		},
		{
			name:             "incorrect current version",
			currentVersion:   "aaa",
			desiredVersion:   "1.20.0",
			err:              errors.New("Malformed version: aaa"),
			isGreaterOrEqual: false,
		},
		{
			name:             "incorrect desired version",
			currentVersion:   "1.20.0",
			desiredVersion:   "bbb",
			err:              errors.New("Malformed version: bbb"),
			isGreaterOrEqual: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			res, err := isCRVersionGreaterOrEqual(tc.currentVersion, tc.desiredVersion)
			if tc.err != nil {
				require.Error(t, err)
				require.Equal(t, err.Error(), tc.err.Error())
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.isGreaterOrEqual, res)
		})
	}
}
