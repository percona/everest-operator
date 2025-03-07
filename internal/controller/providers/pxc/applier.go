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
	"context"
	"errors"
	"fmt"
	"net/url"

	"github.com/AlekSi/pointer"
	pxcv1 "github.com/percona/percona-xtradb-cluster-operator/pkg/apis/pxc/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	everestv1alpha1 "github.com/percona/everest-operator/api/v1alpha1"
	"github.com/percona/everest-operator/internal/controller/common"
)

const (
	haProxyProbesTimeout = 30
)

type applier struct {
	*Provider
	ctx context.Context //nolint:containedctx
}

func (p *applier) Metadata() error {
	if p.PerconaXtraDBCluster.GetDeletionTimestamp().IsZero() {
		for _, f := range []string{
			finalizerDeletePXCPodsInOrder,
			finalizerDeletePXCPVC,
			finalizerDeletePXCSSL,
		} {
			controllerutil.AddFinalizer(p.PerconaXtraDBCluster, f)
		}
	}
	return nil
}

func (p *applier) Paused(paused bool) {
	p.Provider.PerconaXtraDBCluster.Spec.Pause = paused
}

//nolint:staticcheck //using deprecated field for backward compatibility
func (p *applier) AllowUnsafeConfig() {
	p.PerconaXtraDBCluster.Spec.AllowUnsafeConfig = false
	useInsecureSize := p.DB.Spec.Engine.Replicas == 1 || p.DB.Spec.AllowUnsafeConfiguration
	p.PerconaXtraDBCluster.Spec.Unsafe = pxcv1.UnsafeFlags{
		TLS:               p.DB.Spec.AllowUnsafeConfiguration,
		PXCSize:           useInsecureSize,
		ProxySize:         useInsecureSize,
		BackupIfUnhealthy: p.DB.Spec.AllowUnsafeConfiguration,
	}
}

func (p *applier) Engine() error {
	engine := p.DBEngine
	if p.DB.Spec.Engine.Version == "" {
		p.DB.Spec.Engine.Version = engine.BestEngineVersion()
	}

	pxc := p.PerconaXtraDBCluster

	// Update CRVersion, if specified.
	desiredCR := pointer.Get(p.DB.Spec.Engine.CRVersion)
	if desiredCR != "" {
		pxc.Spec.CRVersion = desiredCR
	}

	pxc.Spec.SecretsName = p.DB.Spec.Engine.UserSecretsName
	pxc.Spec.PXC.PodSpec.Size = p.DB.Spec.Engine.Replicas
	pxc.Spec.PXC.PodSpec.Configuration = p.DB.Spec.Engine.Config

	pxcEngineVersion, ok := engine.Status.AvailableVersions.Engine[p.DB.Spec.Engine.Version]
	if !ok {
		return fmt.Errorf("engine version %s not available", p.DB.Spec.Engine.Version)
	}
	pxc.Spec.PXC.Image = pxcEngineVersion.ImagePath

	pxc.Spec.PXC.PodSpec.VolumeSpec = &pxcv1.VolumeSpec{
		PersistentVolumeClaim: &corev1.PersistentVolumeClaimSpec{
			StorageClassName: p.DB.Spec.Engine.Storage.Class,
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: p.DB.Spec.Engine.Storage.Size,
				},
			},
		},
	}

	if !p.DB.Spec.Engine.Resources.CPU.IsZero() {
		pxc.Spec.PXC.PodSpec.Resources.Limits[corev1.ResourceCPU] = p.DB.Spec.Engine.Resources.CPU
		pxc.Spec.PXC.PodSpec.Resources.Requests[corev1.ResourceCPU] = p.DB.Spec.Engine.Resources.CPU
	}
	if !p.DB.Spec.Engine.Resources.Memory.IsZero() {
		pxc.Spec.PXC.PodSpec.Resources.Limits[corev1.ResourceMemory] = p.DB.Spec.Engine.Resources.Memory
		pxc.Spec.PXC.PodSpec.Resources.Requests[corev1.ResourceMemory] = p.DB.Spec.Engine.Resources.Memory
	}
	hasDBSpecChanged := func() bool {
		return p.DB.Status.ObservedGeneration > 0 && p.DB.Status.ObservedGeneration != p.DB.Generation
	}
	// We preserve the settings for existing DBs, otherwise restarts are seen when upgrading Everest.
	// Additionally, we also need to check for the spec changes, otherwise the user can never voluntarily change the resource setting.
	// TODO: Remove this once we figure out how to apply such spec changes without automatic restarts.
	// See: https://perconadev.atlassian.net/browse/EVEREST-1413
	if p.DB.Status.Status == everestv1alpha1.AppStateReady && !hasDBSpecChanged() {
		pxc.Spec.PXC.PodSpec.Resources = p.currentPerconaXtraDBClusterSpec.PXC.PodSpec.Resources
	}

	switch p.DB.Spec.Engine.Size() {
	case everestv1alpha1.EngineSizeSmall:
		pxc.Spec.PXC.PodSpec.LivenessProbes.TimeoutSeconds = 450
		pxc.Spec.PXC.PodSpec.ReadinessProbes.TimeoutSeconds = 450
	case everestv1alpha1.EngineSizeMedium:
		pxc.Spec.PXC.PodSpec.LivenessProbes.TimeoutSeconds = 451
		pxc.Spec.PXC.PodSpec.ReadinessProbes.TimeoutSeconds = 451
	case everestv1alpha1.EngineSizeLarge:
		pxc.Spec.PXC.PodSpec.LivenessProbes.TimeoutSeconds = 600
		pxc.Spec.PXC.PodSpec.ReadinessProbes.TimeoutSeconds = 600
	}
	p.PerconaXtraDBCluster.Spec.PXC.PodSpec.Affinity = &pxcv1.PodAffinity{
		Advanced: common.DefaultAffinitySettings().DeepCopy(),
	}
	// We preserve the settings for existing DBs, otherwise restarts are seen when upgrading Everest.
	// TODO: Remove this once we figure out how to apply such spec changes without automatic restarts.
	// See: https://perconadev.atlassian.net/browse/EVEREST-1413
	if p.DB.Status.Status == everestv1alpha1.AppStateReady {
		pxc.Spec.PXC.PodSpec.Affinity = p.currentPerconaXtraDBClusterSpec.PXC.Affinity
	}
	return nil
}

func (p *applier) Backup() error {
	bkp, err := p.genPXCBackupSpec()
	if err != nil {
		return err
	}
	p.Spec.Backup = bkp
	return nil
}

func (p *applier) Proxy() error {
	proxySpec := p.DB.Spec.Proxy
	// Apply proxy config.
	switch proxySpec.Type {
	case everestv1alpha1.ProxyTypeHAProxy:
		if err := p.applyHAProxyCfg(); err != nil {
			return err
		}
	case everestv1alpha1.ProxyTypeProxySQL:
		if err := p.applyProxySQLCfg(); err != nil {
			return err
		}
	default:
		return fmt.Errorf("invalid proxy type %s", proxySpec.Type)
	}
	return nil
}

func (p *applier) DataSource() error {
	if p.DB.Spec.DataSource == nil {
		// Nothing to do.
		return nil
	}
	// Do not restore from datasource until the cluster is ready.
	if p.DB.Status.Status != everestv1alpha1.AppStateReady {
		return nil
	}
	return common.ReconcileDBRestoreFromDataSource(p.ctx, p.C, p.DB)
}

func (p *applier) Monitoring() error {
	monitoring, err := common.GetDBMonitoringConfig(p.ctx, p.C, p.DB)
	if err != nil {
		return err
	}
	p.PerconaXtraDBCluster.Spec.PMM = defaultSpec().PMM
	if monitoring.Spec.Type == everestv1alpha1.PMMMonitoringType {
		if err := p.applyPMMCfg(monitoring); err != nil {
			return err
		}
	}
	return nil
}

func defaultSpec() pxcv1.PerconaXtraDBClusterSpec {
	maxUnavailable := intstr.FromInt(1)
	return pxcv1.PerconaXtraDBClusterSpec{
		UpdateStrategy: pxcv1.SmartUpdateStatefulSetStrategyType,
		UpgradeOptions: pxcv1.UpgradeOptions{
			Apply:    "never",
			Schedule: "0 4 * * *",
		},
		PXC: &pxcv1.PXCSpec{
			PodSpec: &pxcv1.PodSpec{
				ServiceType: corev1.ServiceTypeClusterIP,
				Affinity: &pxcv1.PodAffinity{
					TopologyKey: pointer.ToString(pxcv1.AffinityTopologyKeyOff),
				},
				PodDisruptionBudget: &pxcv1.PodDisruptionBudgetSpec{
					MaxUnavailable: &maxUnavailable,
				},
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("1G"),
						corev1.ResourceCPU:    resource.MustParse("600m"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("1G"),
						corev1.ResourceCPU:    resource.MustParse("600m"),
					},
				},
			},
		},
		PMM: &pxcv1.PMMSpec{
			Enabled: false,
			Resources: corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("300M"),
					corev1.ResourceCPU:    resource.MustParse("500m"),
				},
			},
		},
		HAProxy: &pxcv1.HAProxySpec{
			PodSpec: pxcv1.PodSpec{
				Enabled: false,
				Affinity: &pxcv1.PodAffinity{
					TopologyKey: pointer.ToString(pxcv1.AffinityTopologyKeyOff),
				},
				Resources: corev1.ResourceRequirements{
					// XXX: Remove this once templates will be available
					Limits: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("1G"),
						corev1.ResourceCPU:    resource.MustParse("600m"),
					},
				},
				ReadinessProbes: corev1.Probe{TimeoutSeconds: haProxyProbesTimeout},
				LivenessProbes:  corev1.Probe{TimeoutSeconds: haProxyProbesTimeout},
			},
		},
		ProxySQL: &pxcv1.ProxySQLSpec{
			PodSpec: pxcv1.PodSpec{
				Enabled: false,
				Affinity: &pxcv1.PodAffinity{
					TopologyKey: pointer.ToString(pxcv1.AffinityTopologyKeyOff),
				},
				Resources: corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("1G"),
						corev1.ResourceCPU:    resource.MustParse("600m"),
					},
				},
			},
		},
	}
}

func (p *applier) applyHAProxyCfg() error {
	haProxy := defaultSpec().HAProxy
	haProxy.PodSpec.Enabled = true
	haProxy.PodSpec.Affinity = &pxcv1.PodAffinity{
		Advanced: common.DefaultAffinitySettings().DeepCopy(),
	}
	// We preserve the settings for existing DBs, otherwise restarts are seen when upgrading Everest.
	// TODO: Remove this once we figure out how to apply such spec changes without automatic restarts.
	// See: https://perconadev.atlassian.net/browse/EVEREST-1413
	if p.DB.Status.Status == everestv1alpha1.AppStateReady {
		haProxy.PodSpec.Affinity = p.currentPerconaXtraDBClusterSpec.HAProxy.PodSpec.Affinity
	}
	switch p.DB.Spec.Engine.Size() {
	case everestv1alpha1.EngineSizeSmall:
		haProxy.PodSpec.Resources = haProxyResourceRequirementsSmall
	case everestv1alpha1.EngineSizeMedium:
		haProxy.PodSpec.Resources = haProxyResourceRequirementsMedium
	case everestv1alpha1.EngineSizeLarge:
		haProxy.PodSpec.Resources = haProxyResourceRequirementsLarge
	}

	if p.DB.Spec.Proxy.Replicas == nil {
		haProxy.PodSpec.Size = p.DB.Spec.Engine.Replicas
	} else {
		haProxy.PodSpec.Size = *p.DB.Spec.Proxy.Replicas
	}

	switch p.DB.Spec.Proxy.Expose.Type {
	case everestv1alpha1.ExposeTypeInternal:
		// No need to set anything, defaults are fine.
	case everestv1alpha1.ExposeTypeExternal:
		annotations := common.ExposeAnnotationsMap[p.clusterType]
		expose := pxcv1.ServiceExpose{
			Enabled:                  true,
			Type:                     corev1.ServiceTypeLoadBalancer,
			LoadBalancerSourceRanges: p.DB.Spec.Proxy.Expose.IPSourceRangesStringArray(),
			Annotations:              annotations,
		}
		haProxy.ExposePrimary = expose
		haProxy.ExposeReplicas = &pxcv1.ReplicasServiceExpose{ServiceExpose: expose}
	default:
		return fmt.Errorf("invalid expose type %s", p.DB.Spec.Proxy.Expose.Type)
	}

	haProxy.PodSpec.Configuration = p.DB.Spec.Proxy.Config
	if haProxy.PodSpec.Configuration == "" {
		haProxy.PodSpec.Configuration = haProxyConfigDefault
	}

	// Ensure there is a env vars secret for HAProxy
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pxcHAProxyEnvSecretName,
			Namespace: p.DB.GetNamespace(),
		},
	}
	if err := controllerutil.SetControllerReference(p.DB, secret, p.C.Scheme()); err != nil {
		return fmt.Errorf("failed to set controller reference for secret %s", pxcHAProxyEnvSecretName)
	}
	if _, err := controllerutil.CreateOrUpdate(p.ctx, p.C, secret, func() error {
		secret.Data = haProxyEnvVars
		return nil
	}); err != nil {
		return fmt.Errorf("failed to create or update secret %w", err)
	}
	haProxy.PodSpec.EnvVarsSecretName = pxcHAProxyEnvSecretName

	haProxyAvailVersions, ok := p.DBEngine.Status.AvailableVersions.Proxy[everestv1alpha1.ProxyTypeHAProxy]
	if !ok {
		return errors.New("haproxy version not available")
	}
	bestHAProxyVersion := haProxyAvailVersions.BestVersion()
	haProxyVersion, ok := haProxyAvailVersions[bestHAProxyVersion]
	if !ok {
		return fmt.Errorf("haproxy version %s not available", bestHAProxyVersion)
	}

	// We can update the HAProxy image name only in case the CRVersions match.
	// Otherwise we keep the image unchanged.
	image := haProxyVersion.ImagePath
	if p.currentPerconaXtraDBClusterSpec.HAProxy != nil && p.DBEngine.Status.OperatorVersion != p.DB.Status.CRVersion {
		image = p.currentPerconaXtraDBClusterSpec.HAProxy.PodSpec.Image
	}
	haProxy.PodSpec.Image = image

	shouldUpdateRequests := shouldUpdateResourceRequests(p.DB.Status.Status)
	if !p.DB.Spec.Proxy.Resources.CPU.IsZero() {
		// When the limits are changed, triggers a pod restart, hence ensuring the requests are applied automatically (next block),
		// as it depends on the cluster being in the 'init' state (shouldUpdateRequests).
		haProxy.PodSpec.Resources.Limits[corev1.ResourceCPU] = p.DB.Spec.Proxy.Resources.CPU
		// Prior to 1.3.0, we did not set the requests, and this led to some issues.
		// We now set the requests to the same value as the limits, however, we need to ensure that
		// they're not automatically applied when Everest is upgraded, otherwise it leads to a proxy restart.
		if shouldUpdateRequests ||
			p.currentPerconaXtraDBClusterSpec.HAProxy.Resources.Requests.Cpu().
				Equal(p.DB.Spec.Proxy.Resources.CPU) {
			haProxy.PodSpec.Resources.Requests[corev1.ResourceCPU] = p.DB.Spec.Proxy.Resources.CPU
		}
	}
	if !p.DB.Spec.Proxy.Resources.Memory.IsZero() {
		// When the limits are changed, triggers a pod restart, hence ensuring the requests are applied automatically (next block),
		// as it depends on the cluster being in the 'init' state (shouldUpdateRequests).
		haProxy.PodSpec.Resources.Limits[corev1.ResourceMemory] = p.DB.Spec.Proxy.Resources.Memory
		// Prior to 1.3.0, we did not set the requests, and this led to some issues.
		// We now set the requests to the same value as the limits, however, we need to ensure that
		// they're not automatically applied when Everest is upgraded, otherwise it leads to a proxy restart.
		if shouldUpdateRequests ||
			p.currentPerconaXtraDBClusterSpec.HAProxy.Resources.Requests.Memory().
				Equal(p.DB.Spec.Proxy.Resources.Memory) {
			haProxy.PodSpec.Resources.Requests[corev1.ResourceMemory] = p.DB.Spec.Proxy.Resources.Memory
		}
	}

	p.PerconaXtraDBCluster.Spec.HAProxy = haProxy
	return nil
}

func shouldUpdateResourceRequests(dbState everestv1alpha1.AppState) bool {
	return dbState == everestv1alpha1.AppStateNew || dbState == everestv1alpha1.AppStateInit
}

func (p *applier) applyProxySQLCfg() error {
	proxySQL := defaultSpec().ProxySQL
	proxySQL.Enabled = true
	proxySQL.Affinity = &pxcv1.PodAffinity{
		Advanced: common.DefaultAffinitySettings().DeepCopy(),
	}
	// We preserve the settings for existing DBs, otherwise restarts are seen when upgrading Everest.
	// TODO: Remove this once we figure out how to apply such spec changes without automatic restarts.
	// See: https://perconadev.atlassian.net/browse/EVEREST-1413
	if p.DB.Status.Status == everestv1alpha1.AppStateReady {
		proxySQL.PodSpec.Affinity = p.currentPerconaXtraDBClusterSpec.ProxySQL.PodSpec.Affinity
	}
	if p.DB.Spec.Proxy.Replicas == nil {
		// By default we set the same number of replicas as the engine
		proxySQL.Size = p.DB.Spec.Engine.Replicas
	} else {
		proxySQL.Size = *p.DB.Spec.Proxy.Replicas
	}

	switch p.DB.Spec.Proxy.Expose.Type {
	case everestv1alpha1.ExposeTypeInternal:
		// No need to set anything, defaults are fine.
	case everestv1alpha1.ExposeTypeExternal:
		expose := pxcv1.ServiceExpose{
			Enabled:                  true,
			Type:                     corev1.ServiceTypeLoadBalancer,
			LoadBalancerSourceRanges: p.DB.Spec.Proxy.Expose.IPSourceRangesStringArray(),
		}
		proxySQL.Expose = expose
	default:
		return fmt.Errorf("invalid expose type %s", p.DB.Spec.Proxy.Expose.Type)
	}

	proxySQL.Configuration = p.DB.Spec.Proxy.Config

	proxySQLAvailVersions, ok := p.DBEngine.Status.AvailableVersions.Proxy[everestv1alpha1.ProxyTypeProxySQL]
	if !ok {
		return errors.New("proxysql version not available")
	}

	bestProxySQLVersion := proxySQLAvailVersions.BestVersion()
	proxySQLVersion, ok := proxySQLAvailVersions[bestProxySQLVersion]
	if !ok {
		return fmt.Errorf("proxysql version %s not available", bestProxySQLVersion)
	}

	// We can update the image name only in case the CRVersions match.
	// Otherwise we keep the image unchanged.
	image := proxySQLVersion.ImagePath
	if p.currentPerconaXtraDBClusterSpec.ProxySQL != nil && p.DBEngine.Status.OperatorVersion != p.DB.Status.CRVersion {
		image = p.currentPerconaXtraDBClusterSpec.ProxySQL.Image
	}
	proxySQL.Image = image

	shouldUpdateRequests := shouldUpdateResourceRequests(p.DB.Status.Status)
	if !p.DB.Spec.Proxy.Resources.CPU.IsZero() {
		// When the limits are changed, triggers a pod restart, hence ensuring the requests are applied automatically (next block),
		// as it depends on the cluster being in the 'init' state (shouldUpdateRequests).
		proxySQL.Resources.Limits[corev1.ResourceCPU] = p.DB.Spec.Proxy.Resources.CPU
		// Prior to 1.3.0, we did not set the requests, and this led to some issues.
		// We now set the requests to the same value as the limits, however, we need to ensure that
		// they're not automatically applied when Everest is upgraded, otherwise it leads to a proxy restart.
		if shouldUpdateRequests ||
			p.currentPerconaXtraDBClusterSpec.HAProxy.Resources.Requests.Cpu().
				Equal(p.DB.Spec.Proxy.Resources.CPU) {
			proxySQL.Resources.Requests[corev1.ResourceCPU] = p.DB.Spec.Proxy.Resources.CPU
		}
	}
	if !p.DB.Spec.Proxy.Resources.Memory.IsZero() {
		// When the limits are changed, triggers a pod restart, hence ensuring the requests are applied automatically (next block),
		// as it depends on the cluster being in the 'init' state (shouldUpdateRequests).
		proxySQL.Resources.Limits[corev1.ResourceMemory] = p.DB.Spec.Proxy.Resources.Memory
		// Prior to 1.3.0, we did not set the requests, and this led to some issues.
		// We now set the requests to the same value as the limits, however, we need to ensure that
		// they're not automatically applied when Everest is upgraded, otherwise it leads to a proxy restart.
		if shouldUpdateRequests ||
			p.currentPerconaXtraDBClusterSpec.HAProxy.Resources.Requests.Cpu().
				Equal(p.DB.Spec.Proxy.Resources.CPU) {
			proxySQL.Resources.Requests[corev1.ResourceMemory] = p.DB.Spec.Proxy.Resources.Memory
		}
	}
	p.PerconaXtraDBCluster.Spec.ProxySQL = proxySQL
	return nil
}

func (p *applier) applyPMMCfg(monitoring *everestv1alpha1.MonitoringConfig) error {
	pxc := p.PerconaXtraDBCluster
	pxc.Spec.PMM.Enabled = true
	image := common.DefaultPMMClientImage
	if monitoring.Spec.PMM.Image != "" {
		image = monitoring.Spec.PMM.Image
	}
	//nolint:godox
	// TODO (K8SPXC-1367): Set PMM container LivenessProbes timeouts once possible.
	pmmURL, err := url.Parse(monitoring.Spec.PMM.URL)
	if err != nil {
		return errors.Join(err, errors.New("invalid monitoring URL"))
	}
	pxc.Spec.PMM.ServerHost = pmmURL.Hostname()
	pxc.Spec.PMM.Image = image
	// Set resources based on cluster size.
	switch p.DB.Spec.Engine.Size() {
	case everestv1alpha1.EngineSizeSmall:
		pxc.Spec.PMM.Resources = pmmResourceRequirementsSmall
	case everestv1alpha1.EngineSizeMedium:
		pxc.Spec.PMM.Resources = pmmResourceRequirementsMedium
	case everestv1alpha1.EngineSizeLarge:
		pxc.Spec.PMM.Resources = pmmResourceRequirementsLarge
	}
	if p.DB.Spec.Monitoring.Resources.Requests != nil {
		pxc.Spec.PMM.Resources.Requests = p.DB.Spec.Monitoring.Resources.Requests
	}
	if p.DB.Spec.Monitoring.Resources.Limits != nil {
		pxc.Spec.PMM.Resources.Limits = p.DB.Spec.Monitoring.Resources.Limits
	}

	apiKey, err := common.GetSecretFromMonitoringConfig(p.ctx, p.C, monitoring)
	if err != nil {
		return err
	}

	err = common.UpdateSecretData(p.ctx, p.C, p.DB, pxc.Spec.SecretsName, map[string][]byte{
		"pmmserverkey": []byte(apiKey),
	})
	if err != nil && !k8serrors.IsNotFound(err) {
		return err
	}
	return nil
}

func (p *applier) genPXCStorageSpec(name, namespace string) (*pxcv1.BackupStorageSpec, *everestv1alpha1.BackupStorage, error) {
	backupStorage := &everestv1alpha1.BackupStorage{}
	err := p.C.Get(p.ctx, types.NamespacedName{Name: name, Namespace: namespace}, backupStorage)
	if err != nil {
		return nil, nil, errors.Join(err, fmt.Errorf("failed to get backup storage %s", name))
	}

	return &pxcv1.BackupStorageSpec{
		Type: pxcv1.BackupStorageType(backupStorage.Spec.Type),
		// XXX: Remove this once templates will be available
		Resources: corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceMemory: resource.MustParse("1G"),
				corev1.ResourceCPU:    resource.MustParse("600m"),
			},
		},
		VerifyTLS: backupStorage.Spec.VerifyTLS,
	}, backupStorage, nil
}

func (p *applier) genPXCBackupSpec() (*pxcv1.PXCScheduledBackup, error) {
	engine := p.DBEngine
	database := p.DB
	// Get the best backup version for the specified database engine
	bestBackupVersion := engine.BestBackupVersion(database.Spec.Engine.Version)
	backupVersion, ok := engine.Status.AvailableVersions.Backup[bestBackupVersion]
	if !ok {
		return nil, fmt.Errorf("backup version %s not available", bestBackupVersion)
	}

	// We can update the image name only in case the CRVersions match.
	// Otherwise we keep the image unchanged.
	image := backupVersion.ImagePath
	if p.currentPerconaXtraDBClusterSpec.Backup != nil && p.DBEngine.Status.OperatorVersion != p.DB.Status.CRVersion {
		image = p.currentPerconaXtraDBClusterSpec.Backup.Image
	}

	// Initialize PXCScheduledBackup object
	pxcBackupSpec := &pxcv1.PXCScheduledBackup{
		Image: image,
		PITR: pxcv1.PITRSpec{
			Enabled: database.Spec.Backup.PITR.Enabled,
		},
	}

	// Initialize map to store backup storages
	storages := make(map[string]*pxcv1.BackupStorageSpec)

	// List DatabaseClusterBackup objects for this database
	backupList, err := common.ListDatabaseClusterBackups(p.ctx, p.C, database.GetName(), database.GetNamespace())
	if err != nil {
		return nil, err
	}

	// Add the storages used by the DatabaseClusterBackup objects
	if err := p.addBackupStorages(backupList, storages); err != nil {
		return nil, err
	}

	// Add PITR configuration if enabled
	if database.Spec.Backup.PITR.Enabled {
		if err := p.addPITRConfiguration(storages, pxcBackupSpec); err != nil {
			return nil, err
		}
	}

	// If there are no schedules, just return the storages used in DatabaseClusterBackup objects
	if len(database.Spec.Backup.Schedules) == 0 {
		pxcBackupSpec.Storages = storages
		return pxcBackupSpec, nil
	}

	// Add scheduled backup configurations
	if err := p.addScheduledBackupsConfiguration(storages, pxcBackupSpec); err != nil {
		return nil, err
	}

	return pxcBackupSpec, nil
}

func (p *applier) addBackupStorages(
	backupList *everestv1alpha1.DatabaseClusterBackupList,
	storages map[string]*pxcv1.BackupStorageSpec,
) error {
	database := p.DB
	for _, backup := range backupList.Items {
		if _, ok := storages[backup.Spec.BackupStorageName]; ok {
			continue
		}

		spec, backupStorage, err := p.genPXCStorageSpec(
			backup.Spec.BackupStorageName,
			database.GetNamespace(),
		)
		if err != nil {
			return errors.Join(err, fmt.Errorf("failed to get backup storage for backup %s", backup.Name))
		}

		storages[backup.Spec.BackupStorageName] = spec

		switch backupStorage.Spec.Type {
		case everestv1alpha1.BackupStorageTypeS3:
			storages[backup.Spec.BackupStorageName].S3 = &pxcv1.BackupStorageS3Spec{
				Bucket: fmt.Sprintf(
					"%s/%s",
					backupStorage.Spec.Bucket,
					common.BackupStoragePrefix(database),
				),
				CredentialsSecret: backupStorage.Spec.CredentialsSecretName,
				Region:            backupStorage.Spec.Region,
				EndpointURL:       backupStorage.Spec.EndpointURL,
			}
		case everestv1alpha1.BackupStorageTypeAzure:
			storages[backup.Spec.BackupStorageName].Azure = &pxcv1.BackupStorageAzureSpec{
				ContainerPath: fmt.Sprintf(
					"%s/%s",
					backupStorage.Spec.Bucket,
					common.BackupStoragePrefix(database),
				),
				CredentialsSecret: backupStorage.Spec.CredentialsSecretName,
			}
		default:
			return fmt.Errorf("unsupported backup storage type %s for %s", backupStorage.Spec.Type, backupStorage.Name)
		}
	}
	return nil
}

func (p *applier) addPITRConfiguration(storages map[string]*pxcv1.BackupStorageSpec, pxcBackupSpec *pxcv1.PXCScheduledBackup) error {
	database := p.DB
	storageName := *database.Spec.Backup.PITR.BackupStorageName

	spec, backupStorage, err := p.genPXCStorageSpec(storageName, database.GetNamespace())
	if err != nil {
		return errors.Join(err, errors.New("failed to get pitr storage"))
	}
	pxcBackupSpec.PITR.StorageName = common.PITRStorageName(storageName)

	var timeBetweenUploads float64
	if database.Spec.Backup.PITR.UploadIntervalSec != nil {
		timeBetweenUploads = float64(*database.Spec.Backup.PITR.UploadIntervalSec)
	}
	pxcBackupSpec.PITR.TimeBetweenUploads = timeBetweenUploads

	switch backupStorage.Spec.Type {
	case everestv1alpha1.BackupStorageTypeS3:
		spec.S3 = &pxcv1.BackupStorageS3Spec{
			Bucket:            common.PITRBucketName(database, backupStorage.Spec.Bucket),
			CredentialsSecret: backupStorage.Spec.CredentialsSecretName,
			Region:            backupStorage.Spec.Region,
			EndpointURL:       backupStorage.Spec.EndpointURL,
		}
	default:
		return fmt.Errorf("BackupStorage of type %s is not supported. PITR only works for s3 compatible storages", backupStorage.Spec.Type)
	}

	// create a separate storage for pxc pitr as the docs recommend
	// https://docs.percona.com/percona-operator-for-mysql/pxc/backups-pitr.html
	storages[common.PITRStorageName(backupStorage.Name)] = spec
	return nil
}

func (p *applier) addScheduledBackupsConfiguration(
	storages map[string]*pxcv1.BackupStorageSpec,
	pxcBackupSpec *pxcv1.PXCScheduledBackup,
) error {
	database := p.DB
	var pxcSchedules []pxcv1.PXCScheduledBackupSchedule //nolint:prealloc
	for _, schedule := range database.Spec.Backup.Schedules {
		if !schedule.Enabled {
			continue
		}

		// Add the storages used by the schedule backups
		if _, ok := storages[schedule.BackupStorageName]; !ok {
			backupStorage := &everestv1alpha1.BackupStorage{}
			err := p.C.Get(p.ctx, types.NamespacedName{
				Name:      schedule.BackupStorageName,
				Namespace: database.GetNamespace(),
			}, backupStorage)
			if err != nil {
				return errors.Join(err, fmt.Errorf("failed to get backup storage %s", schedule.BackupStorageName))
			}

			storages[schedule.BackupStorageName] = &pxcv1.BackupStorageSpec{
				Type:      pxcv1.BackupStorageType(backupStorage.Spec.Type),
				VerifyTLS: backupStorage.Spec.VerifyTLS,
			}
			switch backupStorage.Spec.Type {
			case everestv1alpha1.BackupStorageTypeS3:
				storages[schedule.BackupStorageName].S3 = &pxcv1.BackupStorageS3Spec{
					Bucket: fmt.Sprintf(
						"%s/%s",
						backupStorage.Spec.Bucket,
						common.BackupStoragePrefix(database),
					),
					CredentialsSecret: backupStorage.Spec.CredentialsSecretName,
					Region:            backupStorage.Spec.Region,
					EndpointURL:       backupStorage.Spec.EndpointURL,
				}
			case everestv1alpha1.BackupStorageTypeAzure:
				storages[schedule.BackupStorageName].Azure = &pxcv1.BackupStorageAzureSpec{
					ContainerPath: fmt.Sprintf(
						"%s/%s",
						backupStorage.Spec.Bucket,
						common.BackupStoragePrefix(database),
					),
					CredentialsSecret: backupStorage.Spec.CredentialsSecretName,
				}
			default:
				return fmt.Errorf("unsupported backup storage type %s for %s", backupStorage.Spec.Type, backupStorage.Name)
			}
		}

		pxcSchedules = append(pxcSchedules, pxcv1.PXCScheduledBackupSchedule{
			Name:        schedule.Name,
			Schedule:    schedule.Schedule,
			Keep:        int(schedule.RetentionCopies),
			StorageName: schedule.BackupStorageName,
		})
	}

	pxcBackupSpec.Storages = storages
	pxcBackupSpec.Schedule = pxcSchedules
	return nil
}
