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
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_ParseCSVName(t *testing.T) {
	t.Parallel()
	name, version := parseOperatorCSVName("percona-server-mongodb-operator.v1.0.0")
	assert.Equal(t, "percona-server-mongodb-operator", name)
	assert.Equal(t, "1.0.0", version)

	name, version = parseOperatorCSVName("percona-server-mongodb-operator")
	assert.Equal(t, "", name)
	assert.Equal(t, "", version)

	name, version = parseOperatorCSVName("")
	assert.Equal(t, "", name)
	assert.Equal(t, "", version)
}
