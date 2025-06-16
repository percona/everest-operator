# 🔄 How to: Build Your Own Data Importer

Everest supports restoring databases from external backups using a pluggable system called **data importers**. These allow you to bring your own restore logic — written in any language, using any tool — and plug it into Everest's cluster provisioning workflow.

---

## 🧐 What is a DataImporter?

A **DataImporter** is a reusable, self-contained **blueprint for importing data** into a newly created Everest-managed database cluster.

It defines:

* What container to run (your restore logic)
* Which database engines it supports
* Which input configuration it expects (e.g., S3 path, credentials, custom inputs)
* Any constraints (such as engine version) that your database cluster needs to satisfy 

When Everest provisions a new cluster using a DataImporter, it:

1. Provisions the database cluster.
2. Creates a `DataImportJob` (backed by a Kubernetes Job).
3. Runs your container with a well-defined JSON file describing the import input.
4. Waits for the Job to finish. If successful, the cluster becomes `ready`.

### ✅ Why use a DataImporter?

Because every organization uses different backup tools — `pg_dump`, `mysqldump`, `mongodump`, physical snapshots, or vendor-specific tools — Everest does not try to enforce a one-size-fits-all restore mechanism.

Instead, with `DataImporters`, **you write the logic** for your backup tools, and Everest just runs it for you.

### 🔌 Key Benefits

| Feature       | Benefit                                                 |
| ------------- | ------------------------------------------------------- |
| 🔄 Reusable   | Define once, use across projects and environments.      |
| 🧰 Flexible   | Use any scripting/programming language or restore tool to perform imports. |
| 🔧 Extensible | Supports custom backup formats or workflows.             |
| 🌟 Decoupled  | Everest handles infra; you handle data logic.           |

---

## 📊 1. Understand the DataImporter Contract

Your container will be run with **one command-line argument**: the path to a JSON file containing:

* Source info (e.g., S3 URI, credentials)
* Target DB info (host, port, user, password)
* Optional `config` to tweak your import process (importer-specific)

### Example JSON Input

```json
{
  "source": {
    "path": "/backups/mydb.dump",
    "s3": {
      "bucket": "my-s3-bucket",
      "region": "us-west-2",
      "endpointURL": "https://s3.us-west-2.amazonaws.com",
      "verifyTLS": true,
      "forcePathStyle": false,
      "accessKeyID": "AKIAEXAMPLE",
      "secretKey": "mysecretkey"
    }
  },
  "target": {
    "databaseClusterRef": {
      "name": "my-database-cluster",
      "namespace": "everest-databases"
    },
    "type": "postgresql",
    "host": "my-db-host.everest-databases.svc.cluster.local",
    "port": "5432",
    "user": "dbuser",
    "password": "supersecret"
  },
  "config": {
    "dropIfExists": true,
    "schemaOnly": false,
    "additionalOption": "value"
  }
}

```

The complete contract definition can be found [here](../../api/v1alpha1/dataimporterspec/schema.yaml).

---

## ⚙️ 2. Write Your Importer Script

Use any language. Just make sure it:

* Parses the input JSON file
* Handles the backup (e.g., download from S3)
* Restores it into the target DB
* Exits with status `0` on success, or non-zero on failure

### Example: Python script for importing backups taken with `pg_dump`

```python
#!/usr/bin/env python3
import sys, json, subprocess

# Accept 1 argument - path to a JSON file containing restore input
input_path = sys.argv[1]
with open(input_path) as f:
    config = json.load(f)

source = config["source"]
target = config["target"]

# Download backup from S3
subprocess.check_call([
    "aws", "s3", "cp",
    f"s3://{source['s3']['bucket']}/{source['path']}",
    "dump.sql",
    "--region", source['s3']['region'],
    "--endpoint-url", source['s3']['endpointURL']
])

# Restore into PostgreSQL
subprocess.check_call([
    "psql",
    "-h", target["host"],
    "-p", str(target["port"]),
    "-U", target["username"],
    "-f", "dump.sql"
])
```

---

## 📦 3. Package It as a Docker Image

```Dockerfile
FROM python:3.10
COPY importer.py /importer.py
COPY requirements.txt requirements.txt
RUN ["pip3", "install", "-r", "requirements.txt"]
ENTRYPOINT ["python3", "/importer.py"]
```

Build and push:

```bash
docker build -t myorg/pg-dump-importer .
docker push myorg/pg-dump-importer
```

---

## 📄 4. Register Your Importer in Everest

Create a `DataImporter` CRD:

```yaml
apiVersion: everest.percona.com/v1alpha1
kind: DataImporter
metadata:
  name: pg-dump-importer
spec:
  displayName: "PostgreSQL pg_dump Importer"
  description: "Restores logical backups taken using pg_dump"
  supportedEngines:
    - postgresql
  jobSpec:
    image: myorg/pg-dump-importer
    command: ["python3", "/importer.py"]
```

Optional: define a config schema for `params`:

```yaml
spec:
  config:
    openAPIV3Schema:
      properties:
        my-custom-option:
          type: string
      required: ["my-custion-option"]
```

---

## 🥪 5. Use it to create a new DatabaseCluster

In your `DatabaseCluster` spec:

```yaml
apiVersion: everest.percona.com/v1alpha1
kind: DatabaseCluster
metadata:
  name: my-imported-db
spec:
  # --- hidden ---
  dataSource:
    dataImport:
      spec:
        dataImporterName: pg-dump-importer
        source:
          path: my-backup.sql
          s3:
            bucket: my-bucket
            region: us-west-2
            endpointURL: https://s3.us-west-2.amazonaws.com
            credentialsSecretName: s3-secret
            accessKeyId: my-s3-access-key-id
            secretAccessKey: my-secret-access-key
        # optional, if defined in DataImporter
        config:
          my-custom-config: some-value
```

> NOTE: Upon creation, Everest will hide the `accessKeyId` and `secretAccesKey` fields from this CR and move them to the Secret defined in `credentialsSecretName`. Alternatively, you may directly define `AWS_ACCESS_KEY_ID` and `AWS_SECRET_ACCESS_KEY` in your Secret specified in `credentialsSecretName`.

Everest will:

* Create the cluster
* Run your importer
* Mark the cluster `Ready` when the import succeeds

---

## Accessing the Kubernetes API

If your data import container needs to interact with the Kubernetes API — for example, to create Secrets, ConfigMaps, or Custom Resources — it must explicitly declare the required permissions.

You can define these permissions in the `spec.permissions` and `spec.clusterPermissions` fields of your DatabaseCluster resource. These permissions will be used to generate a Kubernetes Role or ClusterRole (along with a ServiceAccount and RoleBinding or ClusterRoleBinding) that your import job shall use.

Here's an example configuration:

```yaml
apiVersion: everest.percona.com/v1alpha1
kind: DatabaseCluster
metadata:
  name: my-imported-db
spec:
  permissions:
  - apiGroups:
    - pgv2.percona.com
    resources:
    - perconapgrestores
    verbs:
    - create
    - update
    - get
    - delete
  - apiGroups:
    - ""
    resources:
    - secrets
    verbs:
    - get
    - create
    - update
    - delete
  clusterPermissions:
  # -- similar permissions but cluster-wide
```

> 💡 Use `permissions` for namespace-scoped access and `clusterPermissions` for cluster-wide access.

Make sure to include only the minimum necessary permissions to follow the principle of least privilege.

## 🔒 Best Practices

* ✅ Log useful info to `stdout`, and errors to `stderr`
* ✅ Exit with non-zero status on failure
* ✅ Keep container dependencies minimal
* ✅ Document expected `params` and restore behavior
* ✅ Test the image locally before registering

---

## 🧠 Troubleshooting Tips

| Problem          | Check                                       |
| ---------------- | ------------------------------------------- |
| Job fails        | View logs from the Job pod                  |
| Backup not found | Validate S3 path and credentials            |
| Restore fails    | Confirm compatibility with DB version       |
| Params missing   | Ensure required fields are passed correctly |
