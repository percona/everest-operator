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

// Package predicates provides predicates for filtering events.
package predicates

import (
	"context"
	"slices"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
)

// NamespaceFilter is a predicate that filters events based on the specified
// namespace configuration.
type NamespaceFilter struct {
	// AllowNamespaces is a list of namespaces to allow.
	AllowNamespaces []string
	// MatchLabels is a map of labels to match on the namespace.
	// All labels specified here need to match the labels on the namespace (i.e, they're ANDed).
	MatchLabels map[string]string
	// GetNamespace gets the namespace by name.
	GetNamespace func(ctx context.Context, name string) (*corev1.Namespace, error)
	// Log is the logger.
	Log logr.Logger
}

func (p *NamespaceFilter) filterNamespace(namespace *corev1.Namespace) bool {
	if slices.Contains(p.AllowNamespaces, namespace.GetName()) {
		return true
	}
	labels := namespace.GetLabels()
	for k, v := range p.MatchLabels {
		val, ok := labels[k]
		if !ok || val != v {
			return false
		}
	}
	return true
}

func (p *NamespaceFilter) filterObject(obj client.Object) bool {
	if obj.GetNamespace() == "" {
		return true // cluster-scoped resource, always allow.
	}
	namespace, err := p.GetNamespace(context.Background(), obj.GetNamespace())
	if err != nil {
		p.Log.Error(err, "GetNamespace failed", "namespace", obj.GetNamespace())
		return false
	}
	return p.filterNamespace(namespace)
}

// Create returns true if the object should be created.
func (p NamespaceFilter) Create(event event.CreateEvent) bool {
	return p.filterObject(event.Object)
}

// Update returns true if the object should be updated.
func (p NamespaceFilter) Update(event event.UpdateEvent) bool {
	return p.filterObject(event.ObjectNew)
}

// Delete returns true if the object should be deleted.
func (p NamespaceFilter) Delete(event event.DeleteEvent) bool {
	return p.filterObject(event.Object)
}

// Generic returns true if the object should be processed.
func (p NamespaceFilter) Generic(event event.GenericEvent) bool {
	return p.filterObject(event.Object)
}
