#!/bin/bash

set -e

cleanup_db_resource() {
  local resource_name=$1
  
  echo "Cleaning up resource '$resource_name'..."
  if kubectl api-resources | grep -q "$resource_name"; then
    namespaces=$(kubectl get $resource_name -A -o jsonpath='{.items[*].metadata.namespace}' 2>/dev/null || echo "")
    for namespace in $namespaces
    do
      kubectl -n $namespace get $resource_name -o name | awk -F '/' {'print $2'} | xargs --no-run-if-empty kubectl patch $resource_name -n $namespace -p '{"metadata":{"finalizers":null}}' --type merge
    done
  fi
}

cleanup_db_resource "pxc"
cleanup_db_resource "pg"
cleanup_db_resource "psmdb"
cleanup_db_resource "databaseclusters"
  
# Clean up backupstorage resources
echo "Cleaning up backupstorage resources..."
if kubectl api-resources | grep -q "backupstorage"; then
  kubectl delete backupstorage --all-namespaces --all --ignore-not-found=true
fi

# Clean up kuttl namespaces
echo "Cleaning up kuttl namespaces..."
kubectl get ns -o name | grep kuttl | awk -F '/' {'print $2'} | xargs --no-run-if-empty kubectl delete ns

echo "Cleaning up operator namespaces and CRDs..."
kubectl delete ns operators olm --ignore-not-found=true --wait=false

sleep 10
kubectl delete apiservice v1.packages.operators.coreos.com --ignore-not-found=true
kubectl get crd -o name | grep .coreos.com$ | awk -F '/' {'print $2'} | xargs --no-run-if-empty kubectl delete crd
kubectl get crd -o name | grep .percona.com$ | awk -F '/' {'print $2'} | xargs --no-run-if-empty kubectl delete crd

echo "Cleanup completed."