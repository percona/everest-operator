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

import "k8s.io/apimachinery/pkg/runtime/schema"

// ClusterType represents the type of the cluster.
type ClusterType string

const (
	// Everest ...
	Everest = "everest"

	// DBClusterRestoreDBClusterNameField is the field in the DatabaseClusterRestore CR.
	DBClusterRestoreDBClusterNameField = ".spec.dbClusterName"

	// DBClusterBackupDBClusterNameField is the field in the DatabaseClusterBackup CR.
	DBClusterBackupDBClusterNameField = ".spec.dbClusterName"
	// DBClusterBackupBackupStorageNameField is the field in the DatabaseClusterBackup CR.
	DBClusterBackupBackupStorageNameField = ".spec.backupStorageName"
	// DataSourceBackupStorageNameField is the field in the DatabaseClusterBackup CR.
	DataSourceBackupStorageNameField = ".spec.dataSource.backupSource.backupStorageName"

	// TopologyKeyHostname is the topology key for hostname.
	TopologyKeyHostname = "kubernetes.io/hostname"

	// PXCDeploymentName is the name of the Percona XtraDB Cluster operator deployment.
	PXCDeploymentName = "percona-xtradb-cluster-operator"
	// PSDeploymentName is the name of the Percona Server for MySQL operator deployment.
	PSDeploymentName = "percona-server-mysql-operator"
	// PSMDBDeploymentName is the name of the Percona Server for MongoDB operator deployment.
	PSMDBDeploymentName = "percona-server-mongodb-operator"
	// PGDeploymentName is the name of the Percona PostgreSQL operator deployment.
	PGDeploymentName = "percona-postgresql-operator"

	// PXCAPIGroup is the API group for Percona XtraDB Cluster.
	PXCAPIGroup = "pxc.percona.com"
	// PSAPIGroup is the API group for Percona Server for MySQL Cluster.
	PSAPIGroup = "ps.percona.com"
	// PSMDBAPIGroup is the API group for Percona Server for MongoDB.
	PSMDBAPIGroup = "psmdb.percona.com"
	// PGAPIGroup is the API group for Percona PostgreSQL.
	PGAPIGroup = "pgv2.percona.com"

	// PerconaXtraDBClusterKind is the kind for Percona XtraDB Cluster.
	PerconaXtraDBClusterKind = "PerconaXtraDBCluster"
	// PerconaServerMySQLKind is the kind for Percona Server for MySQL Cluster.
	PerconaServerMySQLKind = "PerconaServerMySQL"
	// PerconaServerMongoDBKind is the kind for Percona Server for MongoDB.
	PerconaServerMongoDBKind = "PerconaServerMongoDB"
	// PerconaPGClusterKind is the kind for Percona PostgreSQL.
	PerconaPGClusterKind = "PerconaPGCluster"
	// PerconaXtraDBClusterRestoreKind is the kind for Percona XtraDB Cluster restore.
	PerconaXtraDBClusterRestoreKind = "PerconaXtraDBClusterRestore"

	// ClusterTypeEKS represents the EKS cluster type.
	ClusterTypeEKS ClusterType = "eks"
	// ClusterTypeMinikube represents the Minikube cluster type.
	ClusterTypeMinikube ClusterType = "minikube"

	// LabelKubernetesManagedBy is a common label that indicates the resource is managed by a specific operator.
	LabelKubernetesManagedBy = "app.kubernetes.io/managed-by"

	// EverestSecretsPrefix is the prefix for secrets created by Everest.
	EverestSecretsPrefix = "everest-secrets-"
)

// ExposeAnnotationsMap is a map of annotations needed for exposing the database cluster.
var ExposeAnnotationsMap = map[ClusterType]map[string]string{
	ClusterTypeEKS: {
		"service.beta.kubernetes.io/aws-load-balancer-type": "nlb",
	},
}

//noling:gochecknoglobals
var (
	// PXCGVK is the GroupVersionKind for Percona XtraDB Cluster.
	PXCGVK = schema.GroupVersionKind{Group: PXCAPIGroup, Version: "v1", Kind: PerconaXtraDBClusterKind}
	// PXCRGVK is the GroupVersionKind for Percona PostgreSQL.
	PXCRGVK = schema.GroupVersionKind{Group: PXCAPIGroup, Version: "v1", Kind: PerconaXtraDBClusterRestoreKind}
	// PSMDBGVK is the GroupVersionKind for Percona Server for MongoDB.
	PSMDBGVK = schema.GroupVersionKind{Group: PSMDBAPIGroup, Version: "v1", Kind: PerconaServerMongoDBKind}
	// PGGVK is the GroupVersionKind for Percona PostgreSQL.
	PGGVK = schema.GroupVersionKind{Group: PGAPIGroup, Version: "v2", Kind: PerconaPGClusterKind}
)
