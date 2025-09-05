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

package common //nolint:revive,nolintlint

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	everestv1alpha1 "github.com/percona/everest-operator/api/v1alpha1"
	"github.com/percona/everest-operator/internal/consts"
)

func TestReconcileExposureAnnotations(t *testing.T) {
	t.Parallel()
	type testCase struct {
		name                        string
		lbcs                        []client.Object
		services                    []client.ObjectList
		db                          *everestv1alpha1.DatabaseCluster
		existingUpstreamAnnotations map[string]string
		err                         error
		desiredAnnotations          map[string]string
		desiredIgnore               []string
	}
	ctx := context.Background()
	namespace := "test"
	dbName := "db"
	lbcName := "lbc"

	cases := []testCase{
		{
			name:                        "LB service exists and has annotations; exposed DB with no lbc",
			lbcs:                        []client.Object{},
			services:                    servicesList(t, dbName, namespace, map[string]string{"from/svc": "value-from-svc"}, corev1.ServiceTypeLoadBalancer),
			db:                          db(t, dbName, namespace, "", everestv1alpha1.ExposeTypeExternal),
			existingUpstreamAnnotations: map[string]string{},
			err:                         nil,
			desiredIgnore:               []string{"from/svc"},
			desiredAnnotations: map[string]string{
				consts.DefaultEverestExposeServiceAnnotationsKey: consts.DefaultEverestExposeServiceAnnotationsValue,
			},
		},
		{
			name:                        "LB service exists and has no annotations; exposed DB with no lbc",
			lbcs:                        []client.Object{},
			services:                    servicesList(t, dbName, namespace, map[string]string{}, corev1.ServiceTypeLoadBalancer),
			db:                          db(t, dbName, namespace, "", everestv1alpha1.ExposeTypeExternal),
			existingUpstreamAnnotations: map[string]string{},
			err:                         nil,
			desiredIgnore:               []string{},
			desiredAnnotations: map[string]string{
				consts.DefaultEverestExposeServiceAnnotationsKey: consts.DefaultEverestExposeServiceAnnotationsValue,
			},
		},
		{
			name:                        "LB service exists and has no annotations; exposed DB with lbc",
			lbcs:                        lbcs(t, lbcName, map[string]string{"from/lbc": "value-from-lbc"}),
			services:                    servicesList(t, dbName, namespace, map[string]string{}, corev1.ServiceTypeLoadBalancer),
			db:                          db(t, dbName, namespace, lbcName, everestv1alpha1.ExposeTypeExternal),
			existingUpstreamAnnotations: map[string]string{},
			err:                         nil,
			desiredIgnore:               []string{},
			desiredAnnotations: map[string]string{
				"from/lbc": "value-from-lbc",
				consts.DefaultEverestExposeServiceAnnotationsKey: consts.DefaultEverestExposeServiceAnnotationsValue,
			},
		},
		{
			name:                        "LB service exists and has annotations; exposed DB with lbc",
			lbcs:                        lbcs(t, lbcName, map[string]string{"from/lbc": "value-from-lbc"}),
			services:                    servicesList(t, dbName, namespace, map[string]string{"from/svc": "value-from-svc"}, corev1.ServiceTypeLoadBalancer),
			db:                          db(t, dbName, namespace, lbcName, everestv1alpha1.ExposeTypeExternal),
			existingUpstreamAnnotations: map[string]string{},
			err:                         nil,
			desiredIgnore:               []string{"from/svc"},
			desiredAnnotations: map[string]string{
				"from/lbc": "value-from-lbc",
				consts.DefaultEverestExposeServiceAnnotationsKey: consts.DefaultEverestExposeServiceAnnotationsValue,
			},
		},
		{
			name:                        "expose service is not LB; not exposed DB with lbc",
			lbcs:                        lbcs(t, lbcName, map[string]string{"from/lbc": "value-from-lbc"}),
			services:                    servicesList(t, dbName, namespace, map[string]string{"from/svc": "value-from-svc"}, corev1.ServiceTypeClusterIP),
			db:                          db(t, dbName, namespace, lbcName, everestv1alpha1.ExposeTypeInternal),
			existingUpstreamAnnotations: map[string]string{},
			err:                         nil,
			desiredIgnore:               []string{},
			desiredAnnotations: map[string]string{
				"from/lbc": "value-from-lbc",
				consts.DefaultEverestExposeServiceAnnotationsKey: consts.DefaultEverestExposeServiceAnnotationsValue,
			},
		},
		{
			name:                        "expose service is not LB; not exposed DB no lbc",
			lbcs:                        []client.Object{},
			services:                    servicesList(t, dbName, namespace, map[string]string{"from/svc": "value-from-svc"}, corev1.ServiceTypeClusterIP),
			db:                          db(t, dbName, namespace, "", everestv1alpha1.ExposeTypeInternal),
			existingUpstreamAnnotations: map[string]string{},
			err:                         nil,
			desiredIgnore:               []string{},
			desiredAnnotations: map[string]string{
				consts.DefaultEverestExposeServiceAnnotationsKey: consts.DefaultEverestExposeServiceAnnotationsValue,
			},
		},
		{
			name:                        "No expose svc available",
			lbcs:                        []client.Object{},
			services:                    []client.ObjectList{},
			db:                          db(t, dbName, namespace, "", everestv1alpha1.ExposeTypeInternal),
			existingUpstreamAnnotations: map[string]string{},
			err:                         nil,
			desiredIgnore:               []string{},
			desiredAnnotations:          map[string]string{},
		},
		{
			name: "cloud provider adds it's own annotation",
			lbcs: lbcs(t, lbcName, map[string]string{"from/lbc": "value-from-lbc"}),
			services: servicesList(t, dbName, namespace, map[string]string{
				"from/svc":         "value-from-svc", // annotation added by cloud provider
				"last-config-hash": "some-hash",
				"from/lbc":         "value-from-lbc",
				consts.DefaultEverestExposeServiceAnnotationsKey: consts.DefaultEverestExposeServiceAnnotationsValue,
			}, corev1.ServiceTypeLoadBalancer),
			db: db(t, dbName, namespace, lbcName, everestv1alpha1.ExposeTypeExternal),
			existingUpstreamAnnotations: map[string]string{
				"last-config-hash": "some-hash", // annotation added by upstream operator
				"from/lbc":         "value-from-lbc",
				consts.DefaultEverestExposeServiceAnnotationsKey: consts.DefaultEverestExposeServiceAnnotationsValue,
			},
			err: nil,
			desiredIgnore: []string{
				"from/svc", // ignore the cloud provider added annotation
			},
			// expect only the everest-defined annotations
			desiredAnnotations: map[string]string{
				"from/lbc": "value-from-lbc",
				consts.DefaultEverestExposeServiceAnnotationsKey: consts.DefaultEverestExposeServiceAnnotationsValue,
			},
		},
		{
			name:                        "Expose svc is clusterIP, db defines LoadBalancer",
			lbcs:                        []client.Object{},
			services:                    servicesList(t, dbName, namespace, map[string]string{}, corev1.ServiceTypeClusterIP),
			db:                          db(t, dbName, namespace, "", everestv1alpha1.ExposeTypeExternal),
			existingUpstreamAnnotations: map[string]string{},
			err:                         nil,
			desiredIgnore:               []string{},
			desiredAnnotations: map[string]string{
				consts.DefaultEverestExposeServiceAnnotationsKey: consts.DefaultEverestExposeServiceAnnotationsValue,
			},
		},
		{
			name: "Expose svc is clusterIP with the cloud provider's defined annotations; db defines LoadBalancer",
			lbcs: []client.Object{},
			services: servicesList(t, dbName, namespace, map[string]string{
				"from/svc": "value-from-svc", // cloud provider's added annotation
			}, corev1.ServiceTypeClusterIP),
			db:                          db(t, dbName, namespace, "", everestv1alpha1.ExposeTypeExternal),
			existingUpstreamAnnotations: map[string]string{},
			err:                         nil,
			desiredIgnore:               []string{}, // the cloud provider's added annotations are not ignored since the svc type is not LB
			desiredAnnotations: map[string]string{
				consts.DefaultEverestExposeServiceAnnotationsKey: consts.DefaultEverestExposeServiceAnnotationsValue,
			},
		},
		{
			name:                        "Two LB svcs (pxc-like experience) with different cloud provider's defined annotations",
			lbcs:                        []client.Object{},
			services:                    servicesListTwoSvcs(t, dbName, namespace, map[string]string{"a1": "val1"}, map[string]string{"a2": "val2"}, corev1.ServiceTypeLoadBalancer, corev1.ServiceTypeLoadBalancer),
			db:                          db(t, dbName, namespace, "", everestv1alpha1.ExposeTypeExternal),
			existingUpstreamAnnotations: map[string]string{},
			err:                         nil,
			desiredIgnore: []string{
				"a1", "a2", // the values from different services are merged to be ignored
			},
			desiredAnnotations: map[string]string{
				consts.DefaultEverestExposeServiceAnnotationsKey: consts.DefaultEverestExposeServiceAnnotationsValue,
			},
		},
		{
			name:                        "One Lb and one ClusterIP svc (pxc-like experience) with different cloud provider's defined annotations",
			lbcs:                        []client.Object{},
			services:                    servicesListTwoSvcs(t, dbName, namespace, map[string]string{"a1": "val1"}, map[string]string{"a2": "val2"}, corev1.ServiceTypeClusterIP, corev1.ServiceTypeLoadBalancer),
			db:                          db(t, dbName, namespace, "", everestv1alpha1.ExposeTypeExternal),
			existingUpstreamAnnotations: map[string]string{},
			err:                         nil,
			desiredIgnore: []string{
				"a2", // the value from ClusterIP service is not included
			},
			desiredAnnotations: map[string]string{
				consts.DefaultEverestExposeServiceAnnotationsKey: consts.DefaultEverestExposeServiceAnnotationsValue,
			},
		},
		{
			name:                        "Two ClusterIP svcs (pxc-like experience) with different cloud provider's defined annotations",
			lbcs:                        []client.Object{},
			services:                    servicesListTwoSvcs(t, dbName, namespace, map[string]string{"a1": "val1"}, map[string]string{"a2": "val2"}, corev1.ServiceTypeClusterIP, corev1.ServiceTypeClusterIP),
			db:                          db(t, dbName, namespace, "", everestv1alpha1.ExposeTypeExternal),
			existingUpstreamAnnotations: map[string]string{},
			err:                         nil,
			desiredIgnore:               []string{}, // empty because the svc type is not LB, so no annotations are included to ingnore
			desiredAnnotations: map[string]string{
				consts.DefaultEverestExposeServiceAnnotationsKey: consts.DefaultEverestExposeServiceAnnotationsValue,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			client := fakeClient(t, tc.lbcs, tc.services)
			desiredAnnotations, ignore, err := ReconcileExposureAnnotations(ctx, client, tc.db, tc.existingUpstreamAnnotations, "")
			if tc.err == nil {
				require.NoError(t, err)
			} else {
				require.Equal(t, tc.err, err)
			}
			assert.Equal(t, tc.desiredAnnotations, desiredAnnotations)
			assert.ElementsMatch(t, tc.desiredIgnore, ignore)
		})
	}
}

// nolint:ireturn,nolintlint
func fakeClient(t *testing.T, objects []client.Object, lists []client.ObjectList) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	utilruntime.Must(everestv1alpha1.AddToScheme(scheme))
	utilruntime.Must(corev1.AddToScheme(scheme))
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objects...).WithLists(lists...).Build()
}

func db(t *testing.T, dbName, namespace, lbcName string, exposeType everestv1alpha1.ExposeType) *everestv1alpha1.DatabaseCluster { //nolint:unparam
	t.Helper()
	return &everestv1alpha1.DatabaseCluster{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:      dbName,
			Namespace: namespace,
		},
		Spec: everestv1alpha1.DatabaseClusterSpec{
			Proxy: everestv1alpha1.Proxy{
				Expose: everestv1alpha1.Expose{
					Type:                   exposeType,
					LoadBalancerConfigName: lbcName,
				},
			},
		},
	}
}

func servicesList(t *testing.T, dbName, namespace string, annotations map[string]string, serviceType corev1.ServiceType) []client.ObjectList { //nolint:unparam
	t.Helper()
	return []client.ObjectList{&corev1.ServiceList{
		Items: []corev1.Service{
			{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: namespace,
					Labels: map[string]string{
						consts.ExposureSvcLabel: dbName,
						consts.ComponentLabel:   "",
					},
					Annotations: annotations,
				},
				Spec: corev1.ServiceSpec{
					Type: serviceType,
				},
			},
		},
	}}
}

func servicesListTwoSvcs(t *testing.T, dbName, namespace string, annotations1, annotations2 map[string]string, serviceType1, serviceType2 corev1.ServiceType) []client.ObjectList {
	t.Helper()
	return []client.ObjectList{&corev1.ServiceList{
		Items: []corev1.Service{
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "n1",
					Namespace: namespace,
					Labels: map[string]string{
						consts.ExposureSvcLabel: dbName,
						consts.ComponentLabel:   "",
					},
					Annotations: annotations1,
				},
				Spec: corev1.ServiceSpec{
					Type: serviceType1,
				},
			},
			{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "n2",
					Namespace: namespace,
					Labels: map[string]string{
						consts.ExposureSvcLabel: dbName,
						consts.ComponentLabel:   "",
					},
					Annotations: annotations2,
				},
				Spec: corev1.ServiceSpec{
					Type: serviceType2,
				},
			},
		},
	}}
}

func lbcs(t *testing.T, lbcName string, annotations map[string]string) []client.Object { //nolint:unparam
	t.Helper()
	return []client.Object{&everestv1alpha1.LoadBalancerConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name: lbcName,
		},
		Spec: everestv1alpha1.LoadBalancerConfigSpec{
			Annotations: annotations,
		},
	}}
}
