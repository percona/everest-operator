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

package common

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	everestv1alpha1 "github.com/percona/everest-operator/api/v1alpha1"
)

func TestGetOperatorVersion(t *testing.T) {
	t.Parallel()
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "percona-xtradb-cluster-operator",
			Namespace: "super-x",
		},
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Image: "percona/percona-xtradb-cluster-operator:1.12.0",
						},
					},
				},
			},
		},
	}
	cl := fake.NewClientBuilder().WithObjects(deployment).Build()
	s := scheme.Scheme
	s.AddKnownTypes(everestv1alpha1.GroupVersion, &everestv1alpha1.DatabaseCluster{})
	version, err := GetOperatorVersion(
		context.TODO(),
		cl,
		types.NamespacedName{
			Namespace: "super-x",
			Name:      "percona-xtradb-cluster-operator",
		})
	require.NoError(t, err)
	assert.Equal(t, "1.12.0", version.String())
	assert.NotEqual(t, "1.11.0", version.String())

	_, err = GetOperatorVersion(
		context.TODO(),
		cl,
		types.NamespacedName{
			Namespace: "non-existent",
			Name:      "percona-xtradb-cluster-operator",
		})
	require.Error(t, err)
}

func TestMergeMapSimple(t *testing.T) {
	t.Parallel()
	testDst := map[string]interface{}{
		"a": "apple",
		"b": "banana",
	}
	src := map[string]interface{}{
		"a": "avocado",
		"c": "cherry",
	}
	expDst := map[string]interface{}{
		"a": "avocado",
		"b": "banana",
		"c": "cherry",
	}
	err := mergeMap(testDst, src)
	require.NoError(t, err)
	assert.Equal(t, expDst, testDst)
}

func TestMergeMapNested(t *testing.T) {
	t.Parallel()
	testDst := map[string]interface{}{
		"a": "apple",
		"b": "banana",
		"dry": map[string]interface{}{
			"a": "almond",
			"p": "peanut",
		},
	}
	src := map[string]interface{}{
		"a": "avocado",
		"c": "cherry",
		"dry": map[string]interface{}{
			"c": "cashew",
			"p": "pecan",
		},
		"vegetables": map[string]interface{}{
			"a": "aspargus",
			"b": "beet",
		},
	}
	expDst := map[string]interface{}{
		"a": "avocado",
		"b": "banana",
		"c": "cherry",
		"dry": map[string]interface{}{
			"a": "almond",
			"c": "cashew",
			"p": "pecan",
		},
		"vegetables": map[string]interface{}{
			"a": "aspargus",
			"b": "beet",
		},
	}
	err := mergeMap(testDst, src)
	require.NoError(t, err)
	assert.Equal(t, expDst, testDst)
}

func TestMergeMapError(t *testing.T) {
	t.Parallel()
	testDst := map[string]interface{}{
		"dry": map[string]interface{}{
			"vegetables": map[string]interface{}{
				"a": 1,
			},
		},
	}
	src := map[string]interface{}{
		"dry": map[string]interface{}{
			"vegetables": map[string]interface{}{
				"a": "avocado",
			},
		},
	}
	err := mergeMap(testDst, src)
	require.Error(t, err)
}

func TestConvertToBinarySI(t *testing.T) {
	testCases := []struct {
		in  resource.Quantity
		out resource.Quantity
	}{
		{
			in:  resource.MustParse("1G"),
			out: resource.MustParse("1Gi"),
		},
		{
			in:  resource.MustParse("43G"),
			out: resource.MustParse("43Gi"),
		},
		{
			in:  resource.MustParse("17M"),
			out: resource.MustParse("17Mi"),
		},
		{
			in:  resource.MustParse("127T"),
			out: resource.MustParse("127Ti"),
		},
		{
			in:  resource.MustParse("1Gi"),
			out: resource.MustParse("1Gi"),
		},
		{
			in:  resource.MustParse("9Mi"),
			out: resource.MustParse("9Mi"),
		},
	}
	for i, tc := range testCases {
		t.Run(fmt.Sprintf("test-%d", i), func(t *testing.T) {
			result, _ := ConvertToBinarySI(tc.in)
			assert.Equal(t, tc.out.String(), result.String())
		})
	}
}
