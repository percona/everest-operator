#!/bin/sh

namespaces=$(kubectl get pxc -A -o jsonpath='{.items[*].metadata.namespace}')
for namespace in ${namespaces[@]}
do
kubectl -n $namespace get pxc -o name | awk -F '/' {'print $2'} | xargs --no-run-if-empty kubectl patch pxc -n $namespace -p '{"metadata":{"finalizers":null}}' --type merge
done

namespaces=$(kubectl get psmdb -A -o jsonpath='{.items[*].metadata.namespace}')
for namespace in ${namespaces[@]}
do
kubectl -n $namespace get psmdb -o name | awk -F '/' {'print $2'} | xargs --no-run-if-empty kubectl patch psmdb -n $namespace -p '{"metadata":{"finalizers":null}}' --type merge
done

namespaces=$(kubectl get pg -A -o jsonpath='{.items[*].metadata.namespace}')
for namespace in ${namespaces[@]}
do
kubectl -n $namespace get pg -o name | awk -F '/' {'print $2'} | xargs --no-run-if-empty kubectl patch pg -n $namespace -p '{"metadata":{"finalizers":null}}' --type merge
done

namespaces=$(kubectl get db -A -o jsonpath='{.items[*].metadata.namespace}')
for namespace in ${namespaces[@]}
do
kubectl -n $namespace get db -o name | awk -F '/' {'print $2'} | xargs --no-run-if-empty kubectl patch db -n $namespace -p '{"metadata":{"finalizers":null}}' --type merge
done

kubectl delete db --all-namespaces --all --cascade=foreground
kubectl delete pvc --all-namespaces --all
kubectl delete backupstorage --all-namespaces --all
kubectl get ns -o name | grep kuttl  | awk -F '/' {'print $2'} | xargs --no-run-if-empty kubectl delete ns
kubectl delete ns operators olm --ignore-not-found=true --wait=false

sleep 10
kubectl delete apiservice v1.packages.operators.coreos.com --ignore-not-found=true
kubectl get crd -o name | grep .coreos.com$ | awk -F '/' {'print $2'} | xargs --no-run-if-empty kubectl delete crd