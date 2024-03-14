package common

const (
	DefaultPMMClientImage = "percona/pmm-client:2"

	DBTemplateKindAnnotationKey = "everest.percona.com/dbtemplate-kind"
	DBTemplateNameAnnotationKey = "everest.percona.com/dbtemplate-name"

	DBClusterRestoreDBClusterNameField = ".spec.dbClusterName"
	DBClusterBackupDBClusterNameField  = ".spec.dbClusterName"

	TopologyKeyHostname = "kubernetes.io/hostname"

	// Names of deployments of various DB operators.
	PXCDeploymentName   = "percona-xtradb-cluster-operator"
	PSMDBDeploymentName = "percona-server-mongodb-operator"
	PGDeploymentName    = "percona-postgresql-operator"

	// API groups for various operator CRDs.
	PXCAPIGroup   = "pxc.percona.com"
	PSMDBAPIGroup = "psmdb.percona.com"
	PGAPIGroup    = "pgv2.percona.com"

	// Kinds for various operator DB CRDs.
	PerconaXtraDBClusterKind = "PerconaXtraDBCluster"
	PerconaServerMongoDBKind = "PerconaServerMongoDB"
	PerconaPGClusterKind     = "PerconaPGCluster"

	// Known cluster types on which the operator can run.
	ClusterTypeEKS      ClusterType = "eks"
	ClusterTypeMinikube ClusterType = "minikube"

	LabelBackupStorageName = "percona.com/backup-storage-name"

	EverestSecretsPrefix = "everest-secrests-"
)

var (
	ExposeAnnotationsMap = map[ClusterType]map[string]string{
		ClusterTypeEKS: {
			"service.beta.kubernetes.io/aws-load-balancer-type": "nlb",
		},
	}
)
