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
	"slices"
	"strings"

	"github.com/AlekSi/pointer"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
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
	vmAgentResourceName                    = "everest-monitoring"
	monitoringConfigRefNameLabel           = "everest.percona.com/monitoring-config-ref-name"
	monitoringConfigRefNamespaceLabel      = "everest.percona.com/monitoring-config-ref-namespace"
	monitoringConfigSecretCleanupFinalizer = "everest.percona.com/cleanup-secrets"
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

	err := r.Get(ctx, types.NamespacedName{
		Name:      req.NamespacedName.Name,
		Namespace: req.NamespacedName.Namespace,
	},
		mc)
	if err != nil && !k8serrors.IsNotFound(err) {
		logger.Error(err, "unable to fetch MonitoringConfig")
		return ctrl.Result{}, err
	}
	if !mc.GetDeletionTimestamp().IsZero() {
		return ctrl.Result{}, r.cleanupSecrets(ctx, mc)
	}
	// VerifyTLS is set to 'true' by default, if unspecified.
	if mc.Spec.VerifyTLS == nil {
		mc.Spec.VerifyTLS = pointer.To(true)
	}
	if k8serrors.IsNotFound(err) {
		// NotFound cannot be fixed by requeuing so ignore it. During background
		// deletion, we receive delete events from cluster's dependents after
		// cluster is deleted.
		logger.Info("reconciling VMAgent")
		if err := r.reconcileVMAgent(ctx, mc); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, nil
	}

	credentialsSecret := &corev1.Secret{}
	err = r.Get(ctx, types.NamespacedName{
		Name:      mc.Spec.CredentialsSecretName,
		Namespace: mc.GetNamespace(),
	}, credentialsSecret)
	if err != nil {
		logger.Error(err, "unable to fetch Secret")
		return ctrl.Result{}, err
	}

	if metav1.GetControllerOf(credentialsSecret) == nil {
		logger.Info("setting controller references for the secret")
		err = controllerutil.SetControllerReference(mc, credentialsSecret, r.Client.Scheme())
		if err != nil {
			return ctrl.Result{}, err
		}
		if err = r.Update(ctx, credentialsSecret); err != nil {
			return ctrl.Result{}, err
		}
	}

	logger.Info("reconciling VMAgent")
	if err := r.reconcileVMAgent(ctx, mc); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *MonitoringConfigReconciler) reconcileVMAgent(ctx context.Context, mc *everestv1alpha1.MonitoringConfig) error {
	skipTLS := !pointer.Get(mc.Spec.VerifyTLS)
	vmAgentSpec, err := r.genVMAgentSpec(ctx, skipTLS)
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

func (r *MonitoringConfigReconciler) genVMAgentSpec(ctx context.Context, skipTLSVerify bool) (map[string]interface{}, error) {
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

		secretName, err := r.reconcileSecret(ctx, &monitoringConfig)
		if err != nil {
			return nil, errors.Join(err, errors.New("could not reconcile destination secret"))
		}

		u, err := url.Parse(monitoringConfig.Spec.PMM.URL)
		if err != nil {
			return nil, errors.Join(err, errors.New("failed to parse PMM URL"))
		}
		remoteWrite = append(remoteWrite, map[string]interface{}{
			"basicAuth": map[string]interface{}{
				"password": map[string]interface{}{
					"name": secretName,
					"key":  everestv1alpha1.MonitoringConfigCredentialsSecretAPIKeyKey,
				},
				"username": map[string]interface{}{
					"name": secretName,
					"key":  everestv1alpha1.MonitoringConfigCredentialsSecretUsernameKey,
				},
			},
			"tlsConfig": map[string]interface{}{
				"insecureSkipVerify": skipTLSVerify,
			},
			"url": u.JoinPath("victoriametrics/api/v1/write").String(),
		})
	}
	remoteWrite = deduplicateRemoteWrites(remoteWrite)
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

func deduplicateRemoteWrites(remoteWrites []interface{}) []interface{} {
	slices.SortFunc(remoteWrites, func(a, b interface{}) int {
		v1, ok := a.(map[string]interface{})
		if !ok {
			panic("cannot cast to map[string]interface{}")
		}
		v2, ok := b.(map[string]interface{})
		if !ok {
			panic("cannot cast to map[string]interface{}")
		}
		return strings.Compare(v1["url"].(string), v2["url"].(string)) //nolint:forcetypeassert
	})
	remoteWrites = slices.CompactFunc(remoteWrites, func(a, b interface{}) bool {
		v1, ok := a.(map[string]interface{})
		if !ok {
			panic("cannot cast to map[string]interface{}")
		}
		v2, ok := b.(map[string]interface{})
		if !ok {
			panic("cannot cast to map[string]interface{}")
		}
		return v1["url"].(string) == v2["url"].(string) //nolint:forcetypeassert
	})
	return remoteWrites
}

// cleanupSecrets deletes all secrets in the monitoring namespace that belong to the given MonitoringConfig.
func (r *MonitoringConfigReconciler) cleanupSecrets(ctx context.Context, mc *everestv1alpha1.MonitoringConfig) error {
	// List secrets in the monitoring namespace that belong to this MonitoringConfig.
	secrets := &corev1.SecretList{}
	err := r.List(ctx, secrets, &client.ListOptions{
		Namespace: r.monitoringNamespace,
		LabelSelector: labels.SelectorFromSet(map[string]string{
			monitoringConfigRefNameLabel:      mc.GetName(),
			monitoringConfigRefNamespaceLabel: mc.GetNamespace(),
		}),
	})
	if err != nil {
		return err
	}
	for _, secret := range secrets.Items {
		if err := r.Delete(ctx, &secret); err != nil {
			return err
		}
	}
	// Remove the finalizer from the MonitoringConfig.
	if controllerutil.RemoveFinalizer(mc, monitoringConfigSecretCleanupFinalizer) {
		return r.Update(ctx, mc)
	}
	return nil
}

// reconcileSecret copies the source MonitoringConfig secret onto the monitoring namespace.
// Returns the name of the newly created/updated secret.
func (r *MonitoringConfigReconciler) reconcileSecret(ctx context.Context, mc *everestv1alpha1.MonitoringConfig) (string, error) {
	// If the MonitoringConfig is in the monitoring namespace, we don't have to do any additional work.
	// Use the secret as-is.
	if mc.GetNamespace() == r.monitoringNamespace {
		return mc.Spec.CredentialsSecretName, nil
	}

	// Get the secret in the MonitoringConfig namespace.
	src := &corev1.Secret{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      mc.Spec.CredentialsSecretName,
		Namespace: mc.GetNamespace(),
	}, src)
	if err != nil {
		return "", err
	}

	// Create a copy in the monitoring namespace.
	dstSecretName := src.GetName() + "-" + src.GetNamespace()
	dst := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dstSecretName,
			Namespace: r.monitoringNamespace,
		},
	}
	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, dst, func() error {
		dst.Data = src.DeepCopy().Data
		// Attach labels to identify the parent MonitoringConfig.
		// This is useful for garbage collection, since we cannot have cross-namespace OwnerRefs.
		labels := dst.GetLabels()
		if labels == nil {
			labels = make(map[string]string)
		}
		labels[monitoringConfigRefNameLabel] = mc.GetName()
		labels[monitoringConfigRefNamespaceLabel] = mc.GetNamespace()
		dst.SetLabels(labels)
		return nil
	}); err != nil {
		return "", err
	}
	// Add a clean-up finalizer in the parent MonitoringConfig.
	if controllerutil.AddFinalizer(mc, monitoringConfigSecretCleanupFinalizer) {
		return dstSecretName, r.Update(ctx, mc)
	}
	return dstSecretName, nil
}
