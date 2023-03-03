package controllers

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	dbaasv1 "github.com/percona/dbaas-operator/api/v1"
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
	s.AddKnownTypes(dbaasv1.GroupVersion, &dbaasv1.DatabaseCluster{})
	r := &DatabaseReconciler{Client: cl, Scheme: s}
	version, err := r.getOperatorVersion(context.TODO(), types.NamespacedName{
		Namespace: "super-x",
		Name:      "percona-xtradb-cluster-operator",
	})
	assert.NoError(t, err)
	assert.Equal(t, "1.12.0", version.String())
	assert.NotEqual(t, "1.11.0", version.String())

	_, err = r.getOperatorVersion(context.TODO(), types.NamespacedName{
		Namespace: "non-existent",
		Name:      "percona-xtradb-cluster-operator",
	})
	assert.Error(t, err)
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
	assert.NoError(t, err)
	assert.Equal(t, testDst, expDst)
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
	assert.NoError(t, err)
	assert.Equal(t, testDst, expDst)
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
	assert.Error(t, err)
}
