package utils

import (
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	everestv1alpha1 "github.com/percona/everest-operator/api/v1alpha1"
)

// IsEverestReadOnlyObject Checks whether the Everest object is in use.
// Returns true in case "everest.percona.com/readonly-protection" finalizer is present.
func IsEverestReadOnlyObject(obj client.Object) bool {
	return controllerutil.ContainsFinalizer(obj, everestv1alpha1.ReadOnlyFinalizer)
}

// IsEverestObjectInUse Checks whether the Everest object is in use.
// Returns true in case "everest.percona.com/in-use-protection" finalizer is present.
func IsEverestObjectInUse(obj client.Object) bool {
	return controllerutil.ContainsFinalizer(obj, everestv1alpha1.InUseResourceFinalizer)
}
