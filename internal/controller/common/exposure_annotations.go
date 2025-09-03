package common

import (
	"context"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	everestv1alpha1 "github.com/percona/everest-operator/api/v1alpha1"
	"github.com/percona/everest-operator/internal/consts"
)

var errNoSvcFound = errors.New("no expose service found")

// ReconcileExposureAnnotations returns the annotations to apply to the upstream cluster, the list of annotations keys to ignore to avoid overriding and an error
func ReconcileExposureAnnotations(ctx context.Context, c client.Client, db *everestv1alpha1.DatabaseCluster, upstreamAnnotations map[string]string, componentName string) (desiredUpstreamAnnotations map[string]string, ignore []string, err error) {
	serviceAnnotations, err := getExposeServiceAnnotations(ctx, c, db, componentName)
	if err != nil {
		// if there is no service available yet, we shouldn't populate the upstream annotations & ignore
		if errors.Is(err, errNoSvcFound) {
			return map[string]string{}, []string{}, nil
		}
		return nil, nil, err
	}
	everestAnnotations, err := getAnnotations(ctx, c, db)
	if err != nil {
		return nil, nil, err
	}
	desiredUpstreamAnnotations = addDefaultAnnotation(everestAnnotations)

	return desiredUpstreamAnnotations, getUniqueKeys(serviceAnnotations, upstreamAnnotations), nil
}

// getAnnotations returns annotations from the LoadBalancerConfig used in the given DB.
func getAnnotations(
	ctx context.Context,
	c client.Client,
	database *everestv1alpha1.DatabaseCluster,
) (map[string]string, error) {
	lbc, err := GetLoadBalancerConfig(ctx, c, database)
	if err != nil {
		if errors.Is(err, ErrEmptyLbc) {
			return map[string]string{}, nil
		}

		return nil, err
	}

	return lbc.Spec.Annotations, nil
}

// getExposeServiceAnnotations returns the annotations of the expose service
func getExposeServiceAnnotations(ctx context.Context, c client.Client, db *everestv1alpha1.DatabaseCluster, componentName string) (map[string]string, error) {
	svcs := &corev1.ServiceList{}
	err := c.List(ctx, svcs, client.InNamespace(db.GetNamespace()), client.MatchingLabels{
		consts.ExposureSvcLabel: db.GetName(),
		consts.ComponentLabel:   componentName,
	})
	if err != nil {
		return nil, err
	}
	if len(svcs.Items) == 0 {
		return nil, errNoSvcFound
	}

	emptyMap := make(map[string]string)
	exposeType := db.Spec.Proxy.Expose.Type

	switch exposeType {
	// for internal exposure type we don't care about the annotations
	case everestv1alpha1.ExposeTypeInternal:
		return emptyMap, nil
	case everestv1alpha1.ExposeTypeExternal:
		result := make(map[string]string)
		// pxc has two services (primary and replicas), non-sharded psmdb has as many as the number of nodes,
		// so we need to iterate through them and collect all the existing annotations keys
		for _, svc := range svcs.Items {
			// if the service type is already LB, collect the existing annotations
			if svc.Spec.Type == corev1.ServiceTypeLoadBalancer {
				mergeMaps(result, svc.Annotations)
			}
		}
		return result, nil
	default:
		return nil, fmt.Errorf("invalid expose type: %s", exposeType)
	}

	return emptyMap, nil
}

// everest always adds the default annotation to the list to be able to delete all custom annotations
// since the upstream operator does not support it
func addDefaultAnnotation(annotations map[string]string) map[string]string {
	if annotations == nil {
		annotations = make(map[string]string)
	}
	if _, ok := annotations[consts.DefaultEverestExposeServiceAnnotationsKey]; !ok {
		annotations[consts.DefaultEverestExposeServiceAnnotationsKey] = consts.DefaultEverestExposeServiceAnnotationsValue
	}
	return annotations
}

// returns the keys that appears only in minuend
func getUniqueKeys(minuend, subtrahend map[string]string) []string {
	keys := make([]string, 0)
	for k := range minuend {
		if _, found := subtrahend[k]; !found {
			keys = append(keys, k)
		}
	}
	return keys
}

func mergeMaps(dst, src map[string]string) {
	for k, v := range src {
		dst[k] = v
	}
}
