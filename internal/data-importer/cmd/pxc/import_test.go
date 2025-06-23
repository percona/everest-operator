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
package pxc

import (
	"testing"

	"gotest.tools/assert"
)

func TestGetDestinationAndBucket(t *testing.T) {
	testCases := []struct {
		backupPath          string
		bucketName          string
		expectedDestination string
		expectedBucketName  string
	}{
		{
			backupPath:          "/mysql-e6o/6bba92c5-c3f6-4bd6-b8d1-c4a845811faa/mysql-e6o-2025-06-16-11:15:45-full",
			bucketName:          "test-bucket",
			expectedDestination: "s3://test-bucket/mysql-e6o/6bba92c5-c3f6-4bd6-b8d1-c4a845811faa/mysql-e6o-2025-06-16-11:15:45-full",
			expectedBucketName:  "test-bucket/mysql-e6o/6bba92c5-c3f6-4bd6-b8d1-c4a845811faa",
		},
		{
			backupPath:          "mysql-e6o-2025-06-16-11:15:45-full",
			bucketName:          "test-bucket",
			expectedDestination: "s3://test-bucket/mysql-e6o-2025-06-16-11:15:45-full",
			expectedBucketName:  "test-bucket",
		},
	}

	for _, tc := range testCases {
		destination, bucket := getDestinationAndBucket(tc.backupPath, tc.bucketName)
		assert.Equal(t, destination, tc.expectedDestination, "Destination mismatch")
		assert.Equal(t, bucket, tc.expectedBucketName, "Bucket name mismatch")
	}
}
