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

// Package consts provides constants used across the operator.
package consts

const (

	// EverestLabelPrefix is the prefix for all Everest-related labels.
	EverestLabelPrefix = "everest.percona.com/"

	// BackupStorageNameLabel is the label for backup storage name.
	BackupStorageNameLabel = "percona.com/backup-storage-name"
	// KubernetesManagedByLabel is a common label that indicates the resource is managed by a specific operator.
	KubernetesManagedByLabel = "app.kubernetes.io/managed-by"
	// DatabaseClusterNameLabel indicates the name of the database cluster.
	DatabaseClusterNameLabel = "clusterName"
	// MonitoringConfigNameLabel indicates the name of the monitoring configuration.
	MonitoringConfigNameLabel = "monitoringConfigName"
	// PodSchedulingPolicyNameLabel indicates the name of the pod scheduling policy.
	PodSchedulingPolicyNameLabel = "podSchedulingPolicyName"
	// BackupStorageNameLabelTmpl is a template for the backup storage name label.
	BackupStorageNameLabelTmpl = "backupStorage-%s"
	// BackupStorageLabelValue is the value for the backup storage label indicating it is used.
	BackupStorageLabelValue = "used"

	// DataImportJobRefNameLabel is a label used to identify the name of the DataImportJob that owns a cluster-wide resource.
	// This label is used to mark ownership on cluster-wide resources by the DataImportJob.
	// This is needed because DataImportJob cannot own cluster-wide resources like ClusterRole and ClusterRoleBinding,
	// as Kubernetes does not allow cluster-scoped resources to be owned by namespace-scoped resources.
	DataImportJobRefNameLabel = EverestLabelPrefix + "data-import-job-ref-name"
	// DataImportJobRefNamespaceLabel is a label used to identify the namespace of the DataImportJob that owns a cluster-wide resource.
	// This label is used to mark ownership on cluster-wide resources by the DataImportJob.
	// This is needed because DataImportJob cannot own cluster-wide resources like ClusterRole and ClusterRoleBinding,
	// as Kubernetes does not allow cluster-scoped resources to be owned by namespace-scoped resources.
	DataImportJobRefNamespaceLabel = EverestLabelPrefix + "data-import-job-ref-namespace"
	// MonitoringConfigRefNameLabel is a label used to identify the name of the monitoring configuration that owns a resource.
	MonitoringConfigRefNameLabel = EverestLabelPrefix + "monitoring-config-ref-name"
	// MonitoringConfigRefNamespaceLabel is a label used to identify the namespace of the monitoring configuration that owns a resource.
	MonitoringConfigRefNamespaceLabel = EverestLabelPrefix + "monitoring-config-ref-namespace"
)
