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

package common

const (
	// DefaultPMMClientImage is the default image for PMM client.
	DefaultPMMClientImage = "percona/pmm-client:2"

	// DBTemplateKindAnnotationKey is the annotation key for the database template kind.
	DBTemplateKindAnnotationKey = "everest.percona.com/dbtemplate-kind"
	// DBTemplateNameAnnotationKey is the annotation key for the database template name.
	DBTemplateNameAnnotationKey = "everest.percona.com/dbtemplate-name"

	// DBClusterRestoreDBClusterNameField is the field in the DatabaseClusterRestore CR.
	DBClusterRestoreDBClusterNameField = ".spec.dbClusterName"
	// DBClusterBackupDBClusterNameField is the field in the DatabaseClusterBackup CR.
	DBClusterBackupDBClusterNameField = ".spec.dbClusterName"

	// TopologyKeyHostname is the topology key for hostname.
	TopologyKeyHostname = "kubernetes.io/hostname"

	// PXCDeploymentName is the name of the Percona XtraDB Cluster operator deployment.
	PXCDeploymentName = "percona-xtradb-cluster-operator"
	// PSMDBDeploymentName is the name of the Percona Server for MongoDB operator deployment.
	PSMDBDeploymentName = "percona-server-mongodb-operator"
	// PGDeploymentName is the name of the Percona PostgreSQL operator deployment.
	PGDeploymentName = "percona-postgresql-operator"

	// PXCAPIGroup is the API group for Percona XtraDB Cluster.
	PXCAPIGroup = "pxc.percona.com"
	// PSMDBAPIGroup is the API group for Percona Server for MongoDB.
	PSMDBAPIGroup = "psmdb.percona.com"
	// PGAPIGroup is the API group for Percona PostgreSQL.
	PGAPIGroup = "pgv2.percona.com"

	// PerconaXtraDBClusterKind is the kind for Percona XtraDB Cluster.
	PerconaXtraDBClusterKind = "PerconaXtraDBCluster"
	// PerconaServerMongoDBKind is the kind for Percona Server for MongoDB.
	PerconaServerMongoDBKind = "PerconaServerMongoDB"
	// PerconaPGClusterKind is the kind for Percona PostgreSQL.
	PerconaPGClusterKind = "PerconaPGCluster"

	// ClusterTypeEKS represents the EKS cluster type.
	ClusterTypeEKS ClusterType = "eks"
	// ClusterTypeMinikube represents the Minikube cluster type.
	ClusterTypeMinikube ClusterType = "minikube"

	// LabelBackupStorageName is the label for backup storage name.
	LabelBackupStorageName = "percona.com/backup-storage-name"

	// EverestSecretsPrefix is the prefix for secrets created by Everest.
	EverestSecretsPrefix = "everest-secrests-"
)

// ExposeAnnotationsMap is a map of annotations needed for exposing the database cluster.
var ExposeAnnotationsMap = map[ClusterType]map[string]string{
	ClusterTypeEKS: {
		"service.beta.kubernetes.io/aws-load-balancer-type": "nlb",
	},
}
