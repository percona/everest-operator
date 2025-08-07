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

package predicates

import (
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// GetLoadBalancerConfigPredicate returns a predicate that filters events for LoadBalancerConfig resources.
func GetLoadBalancerConfigPredicate() predicate.Funcs {
	return predicate.Funcs{
		// Nothing to process on create events
		CreateFunc: func(_ event.CreateEvent) bool {
			return false
		},

		// Allow update events.
		UpdateFunc: func(_ event.UpdateEvent) bool {
			return true
		},

		// Nothing to process on delete events
		DeleteFunc: func(_ event.DeleteEvent) bool {
			return false
		},

		// Nothing to process on generic events
		GenericFunc: func(_ event.GenericEvent) bool {
			return false
		},
	}
}
