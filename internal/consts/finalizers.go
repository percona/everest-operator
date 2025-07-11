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
	// EverestFinalizerPrefix is the prefix for all Everest operator finalizers.
	EverestFinalizerPrefix = "everest.percona.com/"

	// ForegroundDeletionFinalizer is the finalizer that ensures foreground deletion for the resource.
	ForegroundDeletionFinalizer = "foregroundDeletion"

	// DBBackupCleanupFinalizer is the finalizer for cleaning up DatabaseClusterBackup.
	// Deprecated: We keep this for backward compatibility.
	DBBackupCleanupFinalizer = EverestFinalizerPrefix + "dbb-cleanup"

	// UpstreamClusterCleanupFinalizer is the finalizer for cleaning up the upstream cluster.
	UpstreamClusterCleanupFinalizer = EverestFinalizerPrefix + "upstream-cluster-cleanup"

	// DataImportJobRBACCleanupFinalizer is a finalizer that is used to clean up cluster-wide RBAC resources.
	// This is needed because DataImportJob cannot own cluster-wide resources like ClusterRole and ClusterRoleBinding,
	// as Kubernetes does not allow cluster-scoped resources to be owned by namespace-scoped resources.
	//
	// Deprecated: Use ordered-cleanup finalizer instead. This finalizer is kept for backward compatibility.
	DataImportJobRBACCleanupFinalizer = EverestFinalizerPrefix + "rbac-cleanup"
	// DataImportJobOrderedCleanupFinalizer is a finalizer that is used to clean up DataImportJob resources in a specific order.
	// This finalizer ensures that the Kubernetes Job is deleted before the RBAC resources.
	// This is necessary so that during termination, the Job possesses the necessary permissions to delete its own resources.
	DataImportJobOrderedCleanupFinalizer = EverestFinalizerPrefix + "ordered-cleanup"

	// MonitoringConfigSecretCleanupFinalizer is a finalizer that is used to clean up secrets created by the monitoring configuration.
	MonitoringConfigSecretCleanupFinalizer = EverestFinalizerPrefix + "cleanup-secrets"
	// MonitoringConfigVMAgentFinalizer is a finalizer that is used to clean up VM Agent resources created by the monitoring configuration.
	MonitoringConfigVMAgentFinalizer = EverestFinalizerPrefix + "vmagent"
)
