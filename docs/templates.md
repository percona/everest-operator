# DatabaseCluster Templates
<!-- toc -->
- [Summary](#summary)
- [Design Details](#design-details)
  - [Annotations](#annotations)
  - [Labels](#labels)
- [Examples](#examples)
  - [Disabling Percona XtraDB Cluster Automatic Upgrade](#disabling-percona-xtradb-cluster-automatic-upgrade)
    - [Creating The Template CRD](#creating-the-template-crd)
    - [Adding Read Permissions For The dbaas-operator To Get The PXCTemplateUpgradeOptions CRs](#adding-read-permissions-for-the-dbaas-operator-to-get-the-pxctemplateugradeoptions-crs)
    - [Creating The Template CR](#creating-the-template-cr)
    - [Applying The Template To Existing DB Clusters](#applying-the-template-to-existing-db-clusters)
<!-- /toc -->

DatabaseCluster Templates is a convention between different providers and `dbaas-operator` as a consumer to assemble customized CR object for the specific Database Engine.

## Summary

DatabaseCluster Template provides the ability to customize Database Clusters that different operators would deploy according to the use case, environment, or infrastructure used.

Use Case could be a database cluster with different load patterns: simple reads, heavy writes, 50/50% read/write, number of connections, etc.

Infrastructure also requires different parameters and tunings for the resulting cluster, such as network configuration (load balancing, exposure), storage classes/types, etc.

Environments could combine both categories and affect the configuration of the resulting Database Cluster.

## Design Details

Percona Kubernetes operators control full CRs that include infrastructural and database configurations:

- configs, sizing, backup parameters, cluster parameters (sharding, replsets)
- toleration, affinity, annotations, storage classes, networking

Different personas would like to control various parts of that complete CR. Some would like more control over DBs and DB Clusters (versions, sizing, backups, configs, parameters). In contrast, others would like to control Kubernetes and Cloud infrastructure (networking, storage, secrets) and how that infrastructure is connected to applications (affinity, networking, RBAC).

All those parts are a subset of Kubernetes CR objects supported by operators and Kubernetes add-ons (such as special annotations) and could be defined as templates.
A template should therefore be comprised of a Kubernetes CRD and CR.
- The template CRD defines a subset of the operator CRD fields that will be customized by the template
- The template CR defines the values that will override the default ones defined by the DBaaS operator

!!! note

    The complete CRD of the underlying operator may also be used as the template CRD.

!!! note

    There is no controller for the DatabaseCluster Templates; thus, the responsibility to lifecycle them is offloaded to the external party. If proper cleanup and management are not created, there might be a lot of zombie CRs and CRDs in a cluster.

### Annotations

Annotations that the user should set for the DatabaseCluster CR for `dbaas-operator` that are related to the templates:
- `dbaas.percona.com/dbtemplate-kind: PSMDBtemplate`: is CustomResource (CR) Kind that implements template
- `dbaas.percona.com/dbtemplate-name: prod-app-X-small`: `metadata.name` identifier for CR that provides the template.

If one of those two parameters (kind, name) is not set - `dbaas-operator` wouldn't be able to identify the template and thus would ignore it.

`dbaas-operator` merges all annotations from the DatabaseCluster CR, DatabaseCluster Template.

There could be optional annotations (both in DatabaseCluster CR and DatabaseCluster Template) that are used by SRE and/or specific applications that manage templates, such as:

- `dbaas.percona.com/dbtemplate-origin: pmm`: who created the template: pmm, user, sre, dba, ci
- `dbaas.percona.com/dbtemplate-type: infra`: type of the template: infra, db-conf, net-conf, etc
- `dbaas.percona.com/origin: pmm`: who created CR for `database-operator`: pmm, user, sre, dba, ci
- etc

### Labels

All the labels from the DatabaseCluster Template are merged to the final DB Cluster CR.

In order for PMM to be able to list the templates the following labels must be set in both the template CRD and CR:
- `dbaas.percona.com/template: yes`
- `dbaas.percona.com/engine: pxc`: the underlying DB engine: pxc or psmdb

## Examples

### Customizing The PXC DB configuration

By default, when creating a PXC DB cluster, the DBaaS operator sets PXC DB configuration to the following:

```
[mysqld]
wsrep_provider_options="gcache.size=600M"
```

A DBA may want to customize the PXC DB configuration to his needs. To accomplish this, the DBA can create a template to change that specific field.

#### Creating The Template CRD

By inspecting the [PXC CRD](https://github.com/percona/percona-xtradb-cluster-operator/blob/v1.11.0/deploy/crd.yaml#L6378-L6379) the DBA finds that he/she needs to change the `spec.pxc.configuration` field.
Therefore, he/she creates a template CRD `pxctpl-crd-pxc-configuration.yaml` with just that field.

```yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  annotations:
    controller-gen.kubebuilder.io/version: v0.8.0
  creationTimestamp: null
  name: pxctemplatepxcconfiguration.dbaas.percona.com
  labels:
    dbaas.percona.com/template: "yes"
    dbaas.percona.com/engine: "pxc"
spec:
  group: dbaas.percona.com
  names:
    kind: PXCTemplatePXCConfiguration
    listKind: PXCTemplatePXCConfigurationList
    plural: pxctemplatepxcconfiguration
    singular: pxctemplatepxcconfiguration
  scope: Namespaced
  versions:
  - name: v1
    schema:
      openAPIV3Schema:
        properties:
          apiVersion:
            type: string
          kind:
            type: string
          metadata:
            type: object
          spec:
            properties:
              pxc:
                properties:
                  configuration:
                    type: string
                type: object
            type: object
          status:
            type: object
        type: object
    served: true
    storage: true
```

```sh
$ kubectl apply -f pxctpl-crd-pxc-configuration.yaml
customresourcedefinition.apiextensions.k8s.io/pxctemplatepxcconfiguration.dbaas.percona.com created
```

#### Adding Read Permissions For The dbaas-operator To Get The PXCTemplateUpgradeOptions CRs

In order for the dbaas-operator to apply the template it needs access to the template CRs.

```sh
$ DBAAS_OPERATOR_MANAGER_ROLE=$(kubectl get clusterroles | grep dbaas-operator | grep -v metrics | grep -v proxy | cut -f 1 -d ' '); kubectl get clusterroles/"$DBAAS_OPERATOR_MANAGER_ROLE" -o yaml > dbaas-operator-manager-role.yaml
$ cat <<EOF >>dbaas-operator-manager-role.yaml
- apiGroups:
  - dbaas.percona.com
  resources:
  - pxctemplatepxcconfiguration
  verbs:
  - get
  - list
EOF
$ kubectl apply -f dbaas-operator-manager-role.yaml
clusterrole.rbac.authorization.k8s.io/dbaas-operator-manager-role configured
```

#### Creating The Template CR

The DBA creates a corresponding CR `pxctpl-pxc-config-max-connection-789.yaml` with the desired values.

```yaml
apiVersion: dbaas.percona.com/v1
kind: PXCTemplatePXCConfiguration
metadata:
  name: pxc-config-max-connections-789
  labels:
    dbaas.percona.com/template: "yes"
    dbaas.percona.com/engine: "pxc"
spec:
  pxc:
    configuration: |
      [mysqld]
      max_connections = 789
```

```sh
$ kubectl apply -f pxctpl-pxc-config-max-connection-789.yaml
pxctemplatepxcconfiguration.dbaas.percona.com/pxc-config-max-connections-789 created
```

#### Applying The Template To Existing DB Clusters

To apply the template to an existing DB, the DBA should update the DB cluster CR to include the following annotations.

```yaml
apiVersion: dbaas.percona.com/v1
kind: DatabaseCluster
metadata:
  name: test-pxc-cluster
  annotations:
    dbaas.percona.com/dbtemplate-kind: PXCTemplatePXCConfiguration
    dbaas.percona.com/dbtemplate-name: pxc-config-max-connections-789
...
```

```sh
$ kubectl apply -f databasecluster.yaml
databasecluster.dbaas.percona.com/test-pxc-cluster configured
$ kubectl describe pxc/test-pxc-cluster | grep -A2 'Configuration'
  Configuration: |
    [mysqld]
    max_connections = 789
```

### Enabling Percona XtraDB Cluster Automatic Upgrade

By default, when creating a PXC DB cluster, the DBaaS operator sets the upgrade strategy to never check the official Percona’s Version Service and thus never upgrade the cluster accordingly.
A DBA may want the upgrade process to happen automatically in certain environments. To accomplish this, the DBA can create a template that enables this functionality and applies it to all relevant clusters.

#### Creating The Template CRD

By reading the [PXC operator documentation](https://docs.percona.com/percona-operator-for-mysql/pxc/update.html#automated-upgrade) and by inspecting the [PXC CRD](https://github.com/percona/percona-xtradb-cluster-operator/blob/v1.11.0/deploy/crd.yaml#L8379-L8392) the DBA finds that he/she needs to change the `spec.updateStrategy` and `spec.upgradeOptions` fields.
Therefore, he/she creates a template CRD `pxctpl-crd-upgrade-options.yaml` with just that small subset of fields.

```yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  creationTimestamp: null
  name: pxctemplateupgradeoptions.dbaas.percona.com
  labels:
    dbaas.percona.com/template: "yes"
    dbaas.percona.com/engine: "pxc"
spec:
  group: dbaas.percona.com
  names:
    kind: PXCTemplateUpgradeOptions
    listKind: PXCTemplateUpgradeOptionsList
    plural: pxctemplateupgradeoptions
    singular: pxctemplateupgradeoptions
  scope: Namespaced
  versions:
  - name: v1
    schema:
      openAPIV3Schema:
        properties:
          apiVersion:
            type: string
          kind:
            type: string
          metadata:
            type: object
          spec:
            properties:
              updateStrategy:
                type: string
              upgradeOptions:
                properties:
                  apply:
                    type: string
                  schedule:
                    type: string
                  versionServiceEndpoint:
                    type: string
                type: object
            type: object
          status:
            type: object
        type: object
    served: true
    storage: true
```

```sh
$ kubectl apply -f pxctpl-crd-upgrade-options.yaml
customresourcedefinition.apiextensions.k8s.io/pxctemplateupgradeoptions.dbaas.percona.com created
```

#### Adding Read Permissions For The dbaas-operator To Get The PXCTemplateUpgradeOptions CRs

In order for the dbaas-operator to apply the template it needs access to the template CRs.

```sh
$ DBAAS_OPERATOR_MANAGER_ROLE=$(kubectl get clusterroles | grep dbaas-operator | grep -v metrics | grep -v proxy | cut -f 1 -d ' '); kubectl get clusterroles/"$DBAAS_OPERATOR_MANAGER_ROLE" -o yaml > dbaas-operator-manager-role.yaml
$ cat <<EOF >>dbaas-operator-manager-role.yaml
- apiGroups:
  - dbaas.percona.com
  resources:
  - pxctemplateupgradeoptions
  verbs:
  - get
  - list
EOF
$ kubectl apply -f dbaas-operator-manager-role.yaml
clusterrole.rbac.authorization.k8s.io/dbaas-operator-manager-role configured
```

#### Creating The Template CR

The DBA creates a corresponding CR `pxctpl-enable-automatic-upgrades.yaml` with the desired values.

```yaml
apiVersion: dbaas.percona.com/v1
kind: PXCTemplateUpgradeOptions
metadata:
  name: enable-automatic-upgrades
  labels:
    dbaas.percona.com/template: "yes"
    dbaas.percona.com/engine: "pxc"
spec:
  updateStrategy: SmartUpdate
  upgradeOptions:
    apply: 8.0-recommended
    schedule: 0 4 * * *
```

```sh
$ kubectl apply -f pxctpl-enable-automatic-upgrades.yaml
pxctemplateugradeoptions.dbaas.percona.com/enable-automatic-upgrades created
```

#### Applying The Template To Existing DB Clusters

To apply the template to an existing DB, the DBA should update the DB cluster CR to include the following annotations.

```yaml
apiVersion: dbaas.percona.com/v1
kind: DatabaseCluster
metadata:
  name: test-pxc-cluster
  annotations:
    dbaas.percona.com/dbtemplate-kind: PXCTemplateUpgradeOptions
    dbaas.percona.com/dbtemplate-name: enable-automatic-upgrades
...
```

```sh
$ kubectl apply -f databasecluster.yaml
databasecluster.dbaas.percona.com/test-pxc-cluster configured
$ kubectl describe pxc/test-pxc-cluster | grep -A2 'Update Strategy'
  Update Strategy:    SmartUpdate
  Upgrade Options:
    Apply:    8.0-recommended
    Schedule: 0 4 * * *
```
