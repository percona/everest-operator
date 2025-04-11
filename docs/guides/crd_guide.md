# Using Everest: A Guide to Database Management with Custom Resource Definitions

The Everest operator is a crucial component of Everest, providing a set of CRDs and easy-to-use abstractions to manage your databases declaratively. This guide will walk you through the CRDs provided by Everest operator and how to use them effectively.

> **Note**: While this guide focuses on using Everest through CRDs, we strongly recommend using the Everest UI or API for most operations. The UI/API provides the following benefits:   
> - Built-in validation and guardrails
> - A complete DBaaS-like experience
> - Simplified management of complex operations
> - Enhanced security and access control
> 
> Use CRDs directly only when you need fine-grained control or are integrating with other Kubernetes tools.

## Table of Contents
- [Database Cluster Management](#database-cluster-management)
  - [Basic Database Cluster](#basic-database-cluster)
  - [Engine Types](#engine-types)
    - [DatabaseEngine CRD](#databaseengine-crd)
  - [Exposing Your Database](#exposing-your-database)
- [Backup Storage Configuration](#backup-storage-configuration)
  - [Manual Backups and Restores](#manual-backups-and-restores)
    - [Creating a Manual Backup](#creating-a-manual-backup)
    - [Restoring from a Backup](#restoring-from-a-backup)
    - [Point-in-Time Recovery (PITR)](#point-in-time-recovery-pitr)
- [Monitoring Configuration](#monitoring-configuration)
- [Examples](#examples)
  - [Complete PostgreSQL Cluster Example](#complete-postgresql-cluster-example)
  - [MongoDB Sharded Cluster Example](#mongodb-sharded-cluster-example)
  - [Percona XtraDB Cluster Example](#percona-xtradb-cluster-example)
- [Status Monitoring](#status-monitoring)
- [Troubleshooting](#troubleshooting)

## Database Cluster Management

The core of Everest is the `DatabaseCluster` CRD, which allows you to define and manage database clusters. Here's how to use it:

### Basic Database Cluster

```yaml
apiVersion: everest.percona.com/v1alpha1
kind: DatabaseCluster
metadata:
  name: my-database
  namespace: default
spec:
  engine:
    type: postgresql  # Can be: postgresql, pxc, or psmdb
    version: "15.3"
    replicas: 3
    storage:
      size: 10Gi
      class: standard  # Your storage class
    resources:
      cpu: "1"
      memory: "2G"
```

### Engine Types

Everest supports three database engines:
- `postgresql`: Percona Distribution for PostgreSQL
- `pxc`: Percona XtraDB Cluster (MySQL)
- `psmdb`: Percona Server for MongoDB

#### DatabaseEngine CRD

The `DatabaseEngine` CRD manages the lifecycle of database engines in your Kubernetes cluster. It handles operator installation, upgrades, and version management.

```yaml
apiVersion: everest.percona.com/v1alpha1
kind: DatabaseEngine
metadata:
  name: postgresql-engine
spec:
  type: postgresql
  allowedVersions:  # Optional: restrict to specific versions
    - "15.3"
    - "15.2"
```

The DatabaseEngine status tracks:
- Operator installation state (`not installed`, `installing`, `installed`, `upgrading`)
- Current operator version
- Available database versions and their components
- Pending operator upgrades

Each version can have one of these statuses:
- `recommended`: The preferred version for production use
- `available`: Supported but not the recommended version
- `unavailable`: Version exists but currently can't be used
- `unsupported`: Version that is no longer supported

To check available versions and their status:

```bash
kubectl get dbengine postgresql-engine -o jsonpath='{.status.availableVersions}'
```

> NOTE: Upgrading the operator should be done from the UI or API only.

### Exposing Your Database

You can expose your database service either internally or externally:

```yaml
spec:
  proxy:
    type: haproxy  # or: pgbouncer, proxysql, mongos
    expose:
      type: external
      ipSourceRanges:  # Optional IP whitelist
        - "10.0.0.0/24"
    replicas: 2
```

## Backup Storage Configuration

Before configuring backups, you need to set up a backup storage location. First, create a Kubernetes secret with your cloud storage credentials:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: backup-credentials
type: Opaque
data:
  AWS_SECRET_ACCESS_KEY: <YOUR BASE64 ENCODED KEY>
  AWS_ACCESS_KEY_ID: <YOUR BASE64 ENCODED ID>
```

Then, create a `BackupStorage` CRD that references this secret:

```yaml
apiVersion: everest.percona.com/v1alpha1
kind: BackupStorage
metadata:
  name: my-backup-storage
spec:
  type: s3  # or: azure
  bucket: "my-backup-bucket"
  region: "us-east-1"
  credentialsSecretName: "backup-credentials"
```

Then, configure backups in your DatabaseCluster:

```yaml
spec:
  backup:
    schedules:
      - name: "daily-backup"
        enabled: true
        schedule: "0 0 * * *"  # Daily at midnight
        retentionCopies: 7
        backupStorageName: "my-backup-storage"
    pitr:  # Point-in-Time Recovery
      enabled: true
      backupStorageName: "my-backup-storage"
      uploadIntervalSec: 300  # 5 minutes
```

### Manual Backups and Restores

In addition to scheduled backups, you can create manual backups and perform restores using the `DatabaseClusterBackup` and `DatabaseClusterRestore` CRDs.

#### Creating a Manual Backup

```yaml
apiVersion: everest.percona.com/v1alpha1
kind: DatabaseClusterBackup
metadata:
  name: manual-backup-2024-04-11
spec:
  dbClusterName: my-database
  backupStorageName: my-backup-storage
```

Monitor the backup status:
```bash
kubectl get dbbackup manual-backup-2024-04-11 -o jsonpath='{.status}'
```

The backup will go through these states:
- `Starting`: Initial backup preparation
- `Running`: Backup in progress
- `Succeeded`: Backup completed successfully
- `Failed`: Backup failed

#### Restoring from a Backup

You can restore from a backup in two ways:

1. **Restore from a DatabaseClusterBackup**:
```yaml
apiVersion: everest.percona.com/v1alpha1
kind: DatabaseClusterRestore
metadata:
  name: restore-from-backup
spec:
  dbClusterName: my-database
  dataSource:
    dbClusterBackupName: manual-backup-2024-04-11
```

2. **Restore from a specific backup path**:
```yaml
apiVersion: everest.percona.com/v1alpha1
kind: DatabaseClusterRestore
metadata:
  name: restore-from-path
spec:
  dbClusterName: my-database
  dataSource:
    backupSource:
      path: "/backups/2024-04-11"
      backupStorageName: my-backup-storage
```

3. **Restore to a New Database Cluster**:

To restore a backup to a new database cluster, create a new `DatabaseCluster` with a `dataSource` field that references the backup:

```yaml
apiVersion: everest.percona.com/v1alpha1
kind: DatabaseCluster
metadata:
  name: restored-database
  namespace: default
spec:
  engine:
    type: postgresql  # Must match the backup's engine type
    version: "15.3"   # Must match the backup's version
    replicas: 3
    storage:
      size: 20Gi
      class: standard
    resources:
      cpu: "2"
      memory: "4G"
  proxy:
    type: pgbouncer
    expose:
      type: external
    replicas: 2
  dataSource:
    dbClusterBackupName: manual-backup-2024-04-11  # Name of the backup to restore from
```

Important considerations when restoring to a new cluster:
- The new cluster must use the same database engine type and version as the backup
- The new cluster's storage size should be at least as large as the original cluster
- The new cluster will be created in the same namespace as the backup
- Other configurations (like proxy settings, monitoring, etc.) may be different from the original cluster

#### Point-in-Time Recovery (PITR)

You can perform point-in-time recovery to restore your database to a specific moment:

```yaml
apiVersion: everest.percona.com/v1alpha1
kind: DatabaseClusterRestore
metadata:
  name: pitr-restore
spec:
  dbClusterName: my-database
  dataSource:
    dbClusterBackupName: base-backup
    pitr:
      type: date
      date: "2024-04-11T15:30:00Z"  # UTC timestamp
```

Or restore to the latest available point:
```yaml
apiVersion: everest.percona.com/v1alpha1
kind: DatabaseClusterRestore
metadata:
  name: latest-pitr
spec:
  dbClusterName: my-database
  dataSource:
    dbClusterBackupName: base-backup
    pitr:
      type: latest
```

Monitor the restore status:
```bash
kubectl get dbrestore restore-from-backup -o jsonpath='{.status}'
```

The restore will go through these states:
- `Starting`: Initial restore preparation
- `Restoring`: Restore in progress
- `Succeeded`: Restore completed successfully
- `Failed`: Restore failed

## Monitoring Configuration

Before enabling monitoring for your database clusters, you need to create a `MonitoringConfig` that defines your monitoring setup. Currently, Everest supports PMM (Percona Monitoring and Management) as the monitoring solution.

First, create a secret with your PMM credentials:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: pmm-credentials
type: Opaque
data:
  username: <YOUR BASE64 ENCODED USERNAME>
  apiKey: <YOUR BASE64 ENCODED API KEY>
```

Then create a `MonitoringConfig`:

```yaml
apiVersion: everest.percona.com/v1alpha1
kind: MonitoringConfig
metadata:
  name: my-monitoring-config
spec:
  type: pmm
  credentialsSecretName: pmm-credentials
  pmm:
    url: "https://pmm.example.com"
    image: "percona/pmm-client:2.41.0"  # Optional: specify PMM client version
  verifyTLS: true  # Optional: verify TLS certificates
```

Now you can enable monitoring for your database cluster:

```yaml
spec:
  monitoring:
    monitoringConfigName: "my-monitoring-config"
    resources:
      limits:
        cpu: "200m"
        memory: "200Mi"
      requests:
        cpu: "100m"
        memory: "100Mi"
```

## Examples

### Complete PostgreSQL Cluster Example

```yaml
apiVersion: everest.percona.com/v1alpha1
kind: DatabaseCluster
metadata:
  name: pg-cluster
  namespace: default
spec:
  engine:
    type: postgresql
    version: "15.3"
    replicas: 3
    storage:
      size: 20Gi
      class: standard
    resources:
      cpu: "2"
      memory: "4G"
  proxy:
    type: pgbouncer
    expose:
      type: external
    replicas: 2
  backup:
    schedules:
      - name: "nightly-backup"
        enabled: true
        schedule: "0 2 * * *"
        retentionCopies: 7
        backupStorageName: "pg-backups"
    pitr:
      enabled: true
      backupStorageName: "pg-backups"
  monitoring:
    monitoringConfigName: "pg-monitoring"

---
apiVersion: everest.percona.com/v1alpha1
kind: BackupStorage
metadata:
  name: pg-backups
spec:
  type: s3
  bucket: "my-pg-backups"
  region: "us-east-1"
  credentialsSecretName: "s3-credentials"
```

### MongoDB Sharded Cluster Example

```yaml
apiVersion: everest.percona.com/v1alpha1
kind: DatabaseCluster
metadata:
  name: mongo-cluster
  namespace: default
spec:
  engine:
    type: psmdb
    version: "6.0.5"
    replicas: 3
    storage:
      size: 50Gi
      class: standard
    resources:
      cpu: "2"
      memory: "8G"
  sharding:
    enabled: true
    shards: 3
    configServer:
      replicas: 3
  proxy:
    type: mongos
    expose:
      type: internal
    replicas: 2
  backup:
    schedules:
      - name: "daily-backup"
        enabled: true
        schedule: "0 1 * * *"
        retentionCopies: 7
        backupStorageName: "mongo-backups"
```

### Percona XtraDB Cluster Example

```yaml
apiVersion: everest.percona.com/v1alpha1
kind: DatabaseCluster
metadata:
  name: mysql-cluster
  namespace: default
spec:
  engine:
    type: pxc
    version: "8.0.32"
    replicas: 3
    storage:
      size: 30Gi
      class: standard
    resources:
      cpu: "2"
      memory: "4G"
  proxy:
    type: haproxy
    expose:
      type: external
      ipSourceRanges:
        - "10.0.0.0/16"
    replicas: 2
  backup:
    schedules:
      - name: "weekly-backup"
        enabled: true
        schedule: "0 0 * * 0"
        retentionCopies: 4
        backupStorageName: "mysql-backups"
    pitr:
      enabled: true
      backupStorageName: "mysql-backups"
```

## Status Monitoring

You can monitor the status of your database cluster using:

```bash
kubectl get databasecluster <cluster-name> -o jsonpath='{.status}'
```

The status field will show one of these states:
- `creating`: Initial creation in progress
- `initializing`: Database is being initialized
- `ready`: Database is running and ready
- `error`: An error has occurred
- `paused`: Database is paused
- `upgrading`: Database is being upgraded
- `restoring`: Database is being restored from backup

## Troubleshooting

If your database cluster isn't behaving as expected:

1. Check the cluster status:
   ```bash
   kubectl describe databasecluster <cluster-name>
   ```

2. Check the operator logs:
   ```bash
   kubectl logs -n <operator-namespace> deployment/everest-operator-controller-manager
   ```
