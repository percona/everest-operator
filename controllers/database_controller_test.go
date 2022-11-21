package controllers

import (
	"context"
	"testing"

	dbaasv1 "github.com/percona/dbaas-operator/api/v1"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestGetOperatorVersion(t *testing.T) {
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
	objs := []runtime.Object{deployment}
	cl := fake.NewFakeClient(objs...)
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

	version, err = r.getOperatorVersion(context.TODO(), types.NamespacedName{
		Namespace: "non-existent",
		Name:      "percona-xtradb-cluster-operator",
	})
	assert.Error(t, err)

}
