#!/bin/bash

set -e
echo "Starting cleanup of custom resources..."

# Clean up PXC resources
echo "Cleaning up PXC resources..."
if kubectl api-resources | grep -q "pxc"; then
  namespaces=$(kubectl get pxc -A -o jsonpath='{.items[*].metadata.namespace}' 2>/dev/null || echo "")
  for namespace in $namespaces
  do
    kubectl -n $namespace get pxc -o name | awk -F '/' {'print $2'} | xargs --no-run-if-empty kubectl patch pxc -n $namespace -p '{"metadata":{"finalizers":null}}' --type merge
  done
fi

# Clean up PSMDB resources
echo "Cleaning up PSMDB resources..."
if kubectl api-resources | grep -q "psmdb"; then
  namespaces=$(kubectl get psmdb -A -o jsonpath='{.items[*].metadata.namespace}' 2>/dev/null || echo "")
  for namespace in $namespaces
  do
    kubectl -n $namespace get psmdb -o name | awk -F '/' {'print $2'} | xargs --no-run-if-empty kubectl patch psmdb -n $namespace -p '{"metadata":{"finalizers":null}}' --type merge
  done
fi

# Clean up PG resources
echo "Cleaning up PG resources..."
if kubectl api-resources | grep -q "pg"; then
  namespaces=$(kubectl get pg -A -o jsonpath='{.items[*].metadata.namespace}' 2>/dev/null || echo "")
  for namespace in $namespaces
  do
    kubectl -n $namespace get pg -o name | awk -F '/' {'print $2'} | xargs --no-run-if-empty kubectl patch pg -n $namespace -p '{"metadata":{"finalizers":null}}' --type merge
  done
fi

# Clean up DB resources
echo "Cleaning up DB resources..."
if kubectl api-resources | grep -q "databaseclusters"; then
  namespaces=$(kubectl get db -A -o jsonpath='{.items[*].metadata.namespace}' 2>/dev/null || echo "")
  for namespace in $namespaces
  do
    kubectl -n $namespace get db -o name | awk -F '/' {'print $2'} | xargs --no-run-if-empty kubectl patch db -n $namespace -p '{"metadata":{"finalizers":null}}' --type merge
  done
  
  echo "Deleting DB resources..."
  kubectl delete db --all-namespaces --all --cascade=foreground --ignore-not-found=true
fi

# Clean up additional resources
echo "Cleaning up PVCs..."
kubectl delete pvc --all-namespaces --all --ignore-not-found=true

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

echo "Cleanup completed."