// everest-operator
// Copyright (C) 2022 Percona LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package controllers contains a set of controllers for everest
package controllers

import (
	"sync"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/source"
)

// DynamicWatcher is a wrapper around controller.Controller that allows a thread-safe way to add
// unique named watchers.
type DynamicWatcher struct {
	controller.Controller
	store sync.Map
	log   logr.Logger
}

// NewDynamicWatcher creates a new DynamicWatcher.
func NewDynamicWatcher(log logr.Logger, c controller.Controller) *DynamicWatcher {
	return &DynamicWatcher{
		Controller: c,
		log:        log,
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
	d.log.Info("Added watchers", "name", name)
	d.store.Store(name, struct{}{})
	return nil
}
