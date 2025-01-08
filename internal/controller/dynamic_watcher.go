package controllers

import (
	"sync"

	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// DynamicWatcher is a wrapper around controller.Controller that allows a thread-safe way to add
// unique named watchers.
type DynamicWatcher struct {
	controller.Controller
	store sync.Map
}

// NewDynamicWatcher creates a new DynamicWatcher.
func NewDynamicWatcher(c controller.Controller) *DynamicWatcher {
	return &DynamicWatcher{
		Controller: c,
	}
}

// AddWatchers adds the given sources to the controller, each group unique by name.
func (d *DynamicWatcher) AddWatchers(
	name string,
	sources ...source.Source,
) error {
	_, ok := d.store.Load(name)
	if ok {
		return nil
	}
	for _, src := range sources {
		if err := d.Controller.Watch(src); err != nil {
			return err
		}
	}
	d.store.Store(name, struct{}{})
	return nil
}
