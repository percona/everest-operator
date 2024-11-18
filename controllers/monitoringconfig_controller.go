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

	"github.com/AlekSi/pointer"
	"github.com/VictoriaMetrics/operator/api/operator/v1beta1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	everestv1alpha1 "github.com/percona/everest-operator/api/v1alpha1"
	"github.com/percona/everest-operator/controllers/common"
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
	if client.IgnoreNotFound(err) != nil {
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
	vmAgentSpec, err := r.genVMAgentSpec(ctx)
	if err != nil {
		return err
	}

	vmAgent := &v1beta1.VMAgent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      vmAgentResourceName,
			Namespace: r.monitoringNamespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "everest",
				"everest.percona.com/type":     "monitoring",
			},
		},
		Spec: vmAgentSpec,
	}

	remoteWriteCount := len(vmAgentSpec.RemoteWrite)

	// No remote writes, delete the VMAgent.
	if remoteWriteCount == 0 {
		if err := r.Delete(ctx, vmAgent); client.IgnoreNotFound(err) != nil {
			return errors.Join(err, errors.New("could not delete vmagent"))
		}
		return nil
	}

	if err := controllerutil.SetOwnerReference(mc, vmAgent, r.Scheme); err != nil {
		return errors.Join(err, errors.New("could not set owner reference"))
	}

	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, vmAgent, func() error {
		vmAgent.Spec = vmAgentSpec
		return nil
	})
	return nil
}

func (r *MonitoringConfigReconciler) genVMAgentSpec(ctx context.Context) (v1beta1.VMAgentSpec, error) {
	spec := v1beta1.VMAgentSpec{
		SelectAllByDefault: true,
		CommonApplicationDeploymentParams: v1beta1.CommonApplicationDeploymentParams{
			ExtraArgs: map[string]string{
				"memory.allowedPercent": "40",
			},
		},
		CommonDefaultableParams: v1beta1.CommonDefaultableParams{
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("250m"),
					corev1.ResourceMemory: resource.MustParse("350Mi"),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("500m"),
					corev1.ResourceMemory: resource.MustParse("850Mi"),
				},
			},
		},
	}

	monitoringConfigList := &everestv1alpha1.MonitoringConfigList{}
	err := r.List(ctx, monitoringConfigList, &client.ListOptions{})
	if err != nil {
		return spec, errors.Join(err, errors.New("could not list monitoringconfigs"))
	}

	remoteWrites := make([]v1beta1.VMAgentRemoteWriteSpec, 0, len(monitoringConfigList.Items))
	for _, monitoringConfig := range monitoringConfigList.Items {
		if monitoringConfig.Spec.Type != everestv1alpha1.PMMMonitoringType {
			continue
		}

		secretName, err := r.reconcileSecret(ctx, &monitoringConfig)
		if err != nil {
			return spec, errors.Join(err, errors.New("could not reconcile destination secret"))
		}

		u, err := url.Parse(monitoringConfig.Spec.PMM.URL)
		if err != nil {
			return spec, errors.Join(err, errors.New("failed to parse PMM URL"))
		}
		url := u.JoinPath("victoriametrics/api/v1/write").String()

		// Check if this URL exists.
		if idx := slices.IndexFunc(remoteWrites, func(rw v1beta1.VMAgentRemoteWriteSpec) bool {
			return rw.URL == url
		}); idx < 0 {
			// already exists.
			continue
		}

		remoteWrites = append(remoteWrites, v1beta1.VMAgentRemoteWriteSpec{
			BasicAuth: &v1beta1.BasicAuth{
				Password: corev1.SecretKeySelector{
					Key: everestv1alpha1.MonitoringConfigCredentialsSecretAPIKeyKey,
					LocalObjectReference: corev1.LocalObjectReference{
						Name: secretName,
					},
				},
				Username: corev1.SecretKeySelector{
					Key: everestv1alpha1.MonitoringConfigCredentialsSecretUsernameKey,
					LocalObjectReference: corev1.LocalObjectReference{
						Name: secretName,
					},
				},
			},
			TLSConfig: &v1beta1.TLSConfig{
				InsecureSkipVerify: pointer.Get(monitoringConfig.Spec.VerifyTLS),
			},
			URL: url,
		})
	}
	spec.RemoteWrite = remoteWrites

	// Use the kube-system namespace ID as the k8s_cluster_id label.
	kubeSystemNamespace := &corev1.Namespace{}
	err = r.Get(ctx, types.NamespacedName{Name: "kube-system"}, kubeSystemNamespace)
	if err != nil {
		return spec, errors.Join(err, errors.New("could not get kube-system namespace"))
	}
	spec.ExternalLabels = map[string]string{
		"k8s_cluster_id": string(kubeSystemNamespace.UID),
	}
	return spec, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *MonitoringConfigReconciler) SetupWithManager(mgr ctrl.Manager, monitoringNamespace string) error {
	r.monitoringNamespace = monitoringNamespace
	if err := v1beta1.AddToScheme(mgr.GetScheme()); err != nil {
		return err
	}
	return ctrl.NewControllerManagedBy(mgr).
		For(&everestv1alpha1.MonitoringConfig{}).
		Owns(&v1beta1.VMAgent{}, builder.MatchEveryOwner).
		Watches(&corev1.Namespace{},
			common.EnqueueObjectsInNamespace(r.Client, &everestv1alpha1.MonitoringConfigList{})).
		WithEventFilter(common.DefaultNamespaceFilter).
		Complete(r)
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
