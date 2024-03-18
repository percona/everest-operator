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
	"context"
	"errors"
	"net/url"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	everestv1alpha1 "github.com/percona/everest-operator/api/v1alpha1"
)

const (
	vmAgentResourceName = "everest-monitoring"
)

// MonitoringConfigReconciler reconciles a MonitoringConfig object.
type MonitoringConfigReconciler struct {
	client.Client
	Scheme              *runtime.Scheme
	monitoringNamespace string
}

//+kubebuilder:rbac:groups=everest.percona.com,resources=monitoringconfigs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=everest.percona.com,resources=monitoringconfigs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=everest.percona.com,resources=monitoringconfigs/finalizers,verbs=update
//+kubebuilder:rbac:groups=operator.victoriametrics.com,resources=vmagents,verbs=get;create;update;delete
//+kubebuilder:rbac:groups=core,resources=namespaces,verbs=get

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// Modify the Reconcile function to compare the state specified by
// the DatabaseClusterBackup object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.1/pkg/reconcile
func (r *MonitoringConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	mc := &everestv1alpha1.MonitoringConfig{}
	logger := log.FromContext(ctx)

	err := r.Get(ctx, types.NamespacedName{Name: req.NamespacedName.Name, Namespace: r.monitoringNamespace}, mc)
	if err != nil && !k8serrors.IsNotFound(err) {
		logger.Error(err, "unable to fetch MonitoringConfig")
		return ctrl.Result{}, err
	}
	if k8serrors.IsNotFound(err) {
		// NotFound cannot be fixed by requeuing so ignore it. During background
		// deletion, we receive delete events from cluster's dependents after
		// cluster is deleted.
		logger.Info("reconciling VMAgent")
		if err := r.reconcileVMAgent(ctx); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	credentialsSecret := &corev1.Secret{}
	err = r.Get(ctx, types.NamespacedName{Name: mc.Spec.CredentialsSecretName, Namespace: r.monitoringNamespace}, credentialsSecret)
	if err != nil {
		logger.Error(err, "unable to fetch Secret")
		return ctrl.Result{}, err
	}

	logger.Info("setting controller references for the secret")
	err = controllerutil.SetControllerReference(mc, credentialsSecret, r.Client.Scheme())
	if err != nil {
		return ctrl.Result{}, err
	}
	if err = r.Update(ctx, credentialsSecret); err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("reconciling VMAgent")
	if err := r.reconcileVMAgent(ctx); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *MonitoringConfigReconciler) reconcileVMAgent(ctx context.Context) error {
	vmAgentSpec, err := r.genVMAgentSpec(ctx)
	if err != nil {
		return err
	}

	vmAgentNamespacedName := types.NamespacedName{
		Name:      vmAgentResourceName,
		Namespace: r.monitoringNamespace,
	}

	vmAgent := &unstructured.Unstructured{}
	vmAgent.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "operator.victoriametrics.com",
		Kind:    "VMAgent",
		Version: "v1beta1",
	})
	vmAgent.SetNamespace(vmAgentNamespacedName.Namespace)
	vmAgent.SetName(vmAgentNamespacedName.Name)
	vmAgent.SetLabels(map[string]string{
		"app.kubernetes.io/managed-by": "everest",
		"everest.percona.com/type":     "monitoring",
	})

	vmAgentSpecRemoteWrite, ok := vmAgentSpec["remoteWrite"].([]interface{})
	if !ok {
		return errors.New("could not get remoteWrite from VMAgent spec")
	}
	vmAgentSpecRemoteWriteCount := len(vmAgentSpecRemoteWrite)

	if err := r.Get(ctx, vmAgentNamespacedName, vmAgent); err != nil {
		if !k8serrors.IsNotFound(err) {
			return errors.Join(err, errors.New("could not get vmagent"))
		}

		if vmAgentSpecRemoteWriteCount == 0 {
			// VMAgent CR does not exist and there are no MonitoringConfig CRs
			// so there is nothing to do.
			return nil
		}

		// VMAgent CR does not exist but there are MonitoringConfig CRs so we
		// need to create the VMAgent CR.
		err = unstructured.SetNestedMap(vmAgent.Object, vmAgentSpec, "spec")
		if err != nil {
			return errors.Join(err, errors.New("could not set vmagent spec"))
		}
		if err := r.Create(ctx, vmAgent); err != nil {
			return errors.Join(err, errors.New("could not create vmagent"))
		}

		return nil
	}

	if vmAgentSpecRemoteWriteCount == 0 {
		// VMAgent CR exists but there are no MonitoringConfig CRs so we need
		// to delete the VMAgent CR.
		if err := r.Delete(ctx, vmAgent); err != nil && !k8serrors.IsNotFound(err) {
			return errors.Join(err, errors.New("could not delete vmagent"))
		}
		return nil
	}

	// VMAgent CR exists and there are MonitoringConfig CRs so we need to
	// update the VMAgent CR.
	err = unstructured.SetNestedMap(vmAgent.Object, vmAgentSpec, "spec")
	if err != nil {
		return errors.Join(err, errors.New("could not set vmagent spec"))
	}
	if err := r.Update(ctx, vmAgent); err != nil {
		return errors.Join(err, errors.New("could not update vmagent"))
	}

	return nil
}

func (r *MonitoringConfigReconciler) genVMAgentSpec(ctx context.Context) (map[string]interface{}, error) {
	vmAgentSpec := map[string]interface{}{
		"extraArgs": map[string]interface{}{
			"memory.allowedPercent": "40",
		},
		"podScrapeNamespaceSelector": map[string]interface{}{},
		"podScrapeSelector":          map[string]interface{}{},
		"probeNamespaceSelector":     map[string]interface{}{},
		"probeSelector":              map[string]interface{}{},
		"remoteWrite":                []interface{}{},
		"resources": map[string]interface{}{
			"requests": map[string]interface{}{
				"cpu":    "250m",
				"memory": "350Mi",
			},
			"limits": map[string]interface{}{
				"cpu":    "500m",
				"memory": "850Mi",
			},
		},
		"selectAllByDefault":             true,
		"serviceScrapeNamespaceSelector": map[string]interface{}{},
		"serviceScrapeSelector":          map[string]interface{}{},
		"staticScrapeNamespaceSelector":  map[string]interface{}{},
		"staticScrapeSelector":           map[string]interface{}{},
	}

	monitoringConfigList := &everestv1alpha1.MonitoringConfigList{}
	err := r.List(ctx, monitoringConfigList, &client.ListOptions{})
	if err != nil {
		return nil, errors.Join(err, errors.New("could not list monitoringconfigs"))
	}

	remoteWrite := []interface{}{}
	for _, monitoringConfig := range monitoringConfigList.Items {
		if monitoringConfig.Spec.Type != everestv1alpha1.PMMMonitoringType {
			continue
		}
		u, err := url.Parse(monitoringConfig.Spec.PMM.URL)
		if err != nil {
			return nil, errors.Join(err, errors.New("failed to parse PMM URL"))
		}
		remoteWrite = append(remoteWrite, map[string]interface{}{
			"basicAuth": map[string]interface{}{
				"password": map[string]interface{}{
					"name": monitoringConfig.Spec.CredentialsSecretName,
					"key":  everestv1alpha1.MonitoringConfigCredentialsSecretAPIKeyKey,
				},
				"username": map[string]interface{}{
					"name": monitoringConfig.Spec.CredentialsSecretName,
					"key":  everestv1alpha1.MonitoringConfigCredentialsSecretUsernameKey,
				},
			},
			"tlsConfig": map[string]interface{}{
				"insecureSkipVerify": true,
			},
			"url": u.JoinPath("victoriametrics/api/v1/write").String(),
		})
	}
	vmAgentSpec["remoteWrite"] = remoteWrite

	// Use the kube-system namespace ID as the k8s_cluster_id label.
	kubeSystemNamespace := &corev1.Namespace{}
	err = r.Get(ctx, types.NamespacedName{Name: "kube-system"}, kubeSystemNamespace)
	if err != nil {
		return nil, errors.Join(err, errors.New("could not get kube-system namespace"))
	}
	vmAgentSpec["externalLabels"] = map[string]interface{}{
		"k8s_cluster_id": string(kubeSystemNamespace.UID),
	}

	return vmAgentSpec, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *MonitoringConfigReconciler) SetupWithManager(mgr ctrl.Manager, monitoringNamespace string) error {
	r.monitoringNamespace = monitoringNamespace
	return ctrl.NewControllerManagedBy(mgr).
		For(&everestv1alpha1.MonitoringConfig{}).
		Complete(r)
}
