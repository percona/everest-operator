package predicates

import (
	"context"
	"slices"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

// NewNamespaceFilter returns a predicate that filters events based on the namespace.
func NewNamespaceFilter() *namespacefilter {
	return &namespacefilter{
		enabled: true,
	}
}

type namespacefilter struct {
	enabled         bool
	allowNamespaces []string
	matchLabels     map[string]string
	c               client.Client
	log             logr.Logger
}

// SetClient sets the client.
func (p *namespacefilter) SetClient(c client.Client) {
	p.c = c
}

// SetAllowNamespaces sets the allowed namespaces.
func (p *namespacefilter) SetAllowNamespaces(namespaces []string) {
	p.allowNamespaces = namespaces
}

// SetEnabled sets the enabled flag.
func (p *namespacefilter) SetEnabled(enabled bool) {
	p.enabled = enabled
}

// SetLogger sets the logger.
func (p *namespacefilter) SetLogger(l logr.Logger) {
	p.log = l
}

// SetMatchLabels sets the match labels.
func (p *namespacefilter) SetMatchLabels(labels map[string]string) {
	p.matchLabels = labels
}

func (p *namespacefilter) filterNamespace(namespace *corev1.Namespace) bool {
	if slices.Contains(p.allowNamespaces, namespace.GetName()) {
		return true
	}
	labels := namespace.GetLabels()
	for k, v := range p.matchLabels {
		val, ok := labels[k]
		if !ok || val != v {
			return false
		}
	}
	return true
}

func (p *namespacefilter) filterObject(obj client.Object) bool {
	if !p.enabled {
		return true
	}
	if obj.GetNamespace() == "" {
		return true // cluster-scoped resource, always allow.
	}
	namespace := &corev1.Namespace{}
	err := p.c.Get(context.Background(), types.NamespacedName{
		Name: obj.GetNamespace()}, namespace)
	if err != nil {
		p.log.Error(err, "failed to get namespace", "namespace", obj.GetNamespace())
		return false
	}
	return p.filterNamespace(namespace)
}

// Create returns true if the object should be created.
func (p *namespacefilter) Create(event event.CreateEvent) bool {
	return p.filterObject(event.Object)
}

// Update returns true if the object should be updated.
func (p *namespacefilter) Update(event event.UpdateEvent) bool {
	return p.filterObject(event.ObjectNew)
}

// Delete returns true if the object should be deleted.
func (p *namespacefilter) Delete(event event.DeleteEvent) bool {
	return p.filterObject(event.Object)
}

// Generic returns true if the object should be processed.
func (p *namespacefilter) Generic(event event.GenericEvent) bool {
	return p.filterObject(event.Object)
}
