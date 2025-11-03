#!/bin/bash

set -e
# ------------ CHANGE VARS BELOW -------------
CLUSTER_NAME=mongodb-8ow
NAMESPACE=everest
PRIVATE_BASE_DOMAIN_NAME_SUFFIX=svc.cluster.local
CA_NAME=private
# ----------------------------------------------------

CWD=$(pwd)
CERT_DIR="${CWD}/tls"

[ -d "${CERT_DIR}" ] || mkdir "${CERT_DIR}"

# Functions
function generateCA() {
  cat <<EOF > ca-config-${CA_NAME}.json
{
  "signing": {
    "profiles": {
      "ca": {
        "expiry": "8760h",
        "usages": ["cert sign", "digital signature"]
      },
      "server": {
        "expiry": "8760h",
        "usages": ["digital signature", "key encipherment", "server auth", "client auth"]
      },
      "client": {
        "expiry": "8760h",
        "usages": ["key encipherment", "client auth"]
      }
    }
  }
}
EOF

  cat <<EOF | cfssl gencert -initca -profile=ca - | cfssljson -bare ca-${CA_NAME}
{
  "CN": "Root CA",
  "names": [
    {
      "O": "PSMDB"
    }
  ],
  "key": {
    "algo": "rsa",
    "size": 2048
  }
}
EOF

mv ./ca-* "${CERT_DIR}/"
}

function generateSslServer() {
  cat <<EOF | cfssl gencert -ca="${CERT_DIR}/ca-${CA_NAME}.pem"  -ca-key="${CERT_DIR}/ca-${CA_NAME}-key.pem" -config="${CERT_DIR}/ca-config-${CA_NAME}.json" -profile=server - | cfssljson -bare "${CLUSTER_NAME}-server-${CA_NAME}"
{
  "hosts": [
    "localhost",
    "${CLUSTER_NAME}-rs0",
    "*.${CLUSTER_NAME}-rs0",
    "${CLUSTER_NAME}-rs0.${NAMESPACE}",
    "*.${CLUSTER_NAME}-rs0.${NAMESPACE}",
    "${CLUSTER_NAME}-rs0.${NAMESPACE}.${PRIVATE_BASE_DOMAIN_NAME_SUFFIX}",
    "*.${CLUSTER_NAME}-rs0.${NAMESPACE}.${PRIVATE_BASE_DOMAIN_NAME_SUFFIX}"
  ],
  "names": [
    {
      "O": "PSMDB"
    }
  ],
  "key": {
    "algo": "rsa",
    "size": 2048
  }
}
EOF

mv ${CLUSTER_NAME}-server-${CA_NAME}* "${CERT_DIR}/"
}

function generateSslClient() {
  cat <<EOF | cfssl gencert -ca="${CERT_DIR}/ca-${CA_NAME}.pem"  -ca-key="${CERT_DIR}/ca-${CA_NAME}-key.pem" -config="${CERT_DIR}/ca-config-${CA_NAME}.json" -profile=client - | cfssljson -bare "${CLUSTER_NAME}-client-${CA_NAME}"
{
  "CN": "${CA_NAME} client",
  "hosts": [""],
  "names": [
    {
      "O": "${PRIVATE_BASE_DOMAIN_NAME_SUFFIX}"
    }
  ],
  "key": {
    "algo": "rsa",
    "size": 2048
  }
}
EOF

cfssl bundle -cert "${CLUSTER_NAME}-client-${CA_NAME}.pem" -key  "${CLUSTER_NAME}-client-${CA_NAME}-key.pem" -ca-bundle "${CERT_DIR}/ca-${CA_NAME}.pem"  | cfssljson -bare "${CLUSTER_NAME}-client-bundle-${CA_NAME}"
mv ${CLUSTER_NAME}-client-${CA_NAME}* "${CERT_DIR}/"
}

# main

generateCA
generateSslServer
generateSslClient

# ======== Custom domain ===============

# Connect to MongoDB Instances with client cert validation - WORKS!!!
# NOTE: Works only in case custom TLS is provided during DB cluster creation.

# W/o certs
# NAMESPACE=everest; DB_NAME=mongodb-8ow; DB_PASSWORD=ats7EDzw2KFPvv7H; BASE_DOMAIN_SUFFIX=mycompany.com; ./mongosh mongodb://databaseAdmin:${DB_PASSWORD}@${DB_NAME}-rs0-0.${DB_NAME}-rs0.${NAMESPACE}.${BASE_DOMAIN_SUFFIX}:27017,${DB_NAME}-rs0-1.${DB_NAME}-rs0.${NAMESPACE}.${BASE_DOMAIN_SUFFIX}:27017,${DB_NAME}-rs0-2.${DB_NAME}-rs0.${NAMESPACE}.${BASE_DOMAIN_SUFFIX}:27017

# With certs

# DB
# NAMESPACE=everest; DB_NAME=mongodb-8ow; DB_PASSWORD=C5BJUtbgE9a0cgheggb; BASE_DOMAIN_SUFFIX=mycompany.com; PORT=27017; mongosh --tls --tlsCertificateKeyFile /tmp/shdc-tls-${DB_NAME}.pem --tlsCAFile /etc/shdc-ssl-${DB_NAME}/ca.crt mongodb://databaseAdmin:${DB_PASSWORD}@${DB_NAME}-rs0-0.${DB_NAME}-rs0.${NAMESPACE}.${BASE_DOMAIN_SUFFIX}:${PORT},${DB_NAME}-rs0-1.${DB_NAME}-rs0.${NAMESPACE}.${BASE_DOMAIN_SUFFIX}:${PORT},${DB_NAME}-rs0-2.${DB_NAME}-rs0.${NAMESPACE}.${BASE_DOMAIN_SUFFIX}:${PORT}

# LOCAL
# w/o certs
# svc.cluster.local
# NAMESPACE=everest; DB_NAME=mongodb-8ow; DB_PASSWORD=qm36svGKA2tAT6Vq; BASE_DOMAIN_SUFFIX=svc.cluster.local; PORT=27017; ./mongosh mongodb://databaseAdmin:${DB_PASSWORD}@${DB_NAME}-rs0-0.${DB_NAME}-rs0.${NAMESPACE}.${BASE_DOMAIN_SUFFIX}:${PORT},${DB_NAME}-rs0-1.${DB_NAME}-rs0.${NAMESPACE}.${BASE_DOMAIN_SUFFIX}:${PORT},${DB_NAME}-rs0-2.${DB_NAME}-rs0.${NAMESPACE}.${BASE_DOMAIN_SUFFIX}:${PORT}

# mycompany.com
# NAMESPACE=everest; DB_NAME=mongodb-8ow; DB_PASSWORD=HXXLw8HJaVJ6Egw0X52; BASE_DOMAIN_SUFFIX=mycompany.com; PORT=27017; ./mongosh mongodb://databaseAdmin:${DB_PASSWORD}@${DB_NAME}-rs0-0.${DB_NAME}-rs0.${NAMESPACE}.${BASE_DOMAIN_SUFFIX}:${PORT},${DB_NAME}-rs0-1.${DB_NAME}-rs0.${NAMESPACE}.${BASE_DOMAIN_SUFFIX}:${PORT},${DB_NAME}-rs0-2.${DB_NAME}-rs0.${NAMESPACE}.${BASE_DOMAIN_SUFFIX}:${PORT}

# with certs
# NAMESPACE=everest; DB_NAME=mongodb-8ow; DB_PASSWORD=HXXLw8HJaVJ6Egw0X52; BASE_DOMAIN_SUFFIX=mycompany.com; PORT=27017; CERT_PATH=/Users/yakut/go_path/src/github.com/percona/everest-operator/yakut-certs/tls; ./mongosh --tls --tlsCertificateKeyFile ${CERT_PATH}/${DB_NAME}-client-combined.pem --tlsCAFile ${CERT_PATH}/ca.pem mongodb://databaseAdmin:${DB_PASSWORD}@${DB_NAME}-rs0-0.${DB_NAME}-rs0.${NAMESPACE}.${BASE_DOMAIN_SUFFIX}:${PORT},${DB_NAME}-rs0-1.${DB_NAME}-rs0.${NAMESPACE}.${BASE_DOMAIN_SUFFIX}:${PORT},${DB_NAME}-rs0-2.${DB_NAME}-rs0.${NAMESPACE}.${BASE_DOMAIN_SUFFIX}:${PORT}

# NAMESPACE=everest; DB_NAME=mongodb-8ow; DB_PASSWORD=HXXLw8HJaVJ6Egw0X52; BASE_DOMAIN_SUFFIX=mycompany.com; PORT=27017; mongosh --tls --tlsAllowInvalidCertificates --tlsCertificateKeyFile /tmp/shdc-tls-mongodb-8ow.pem --tlsCAFile /etc/shdc-ssl-mongodb-8ow/ca.crt mongodb://databaseAdmin:${DB_PASSWORD}@${DB_NAME}-rs0-0.${DB_NAME}-rs0.${NAMESPACE}.${BASE_DOMAIN_SUFFIX}:${PORT},${DB_NAME}-rs0-1.${DB_NAME}-rs0.${NAMESPACE}.${BASE_DOMAIN_SUFFIX}:${PORT},${DB_NAME}-rs0-2.${DB_NAME}-rs0.${NAMESPACE}.${BASE_DOMAIN_SUFFIX}:${PORT}