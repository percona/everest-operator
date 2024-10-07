//nolint:revive
package predicates

import (
	"sigs.k8s.io/controller-runtime/pkg/event"
)

// Nop is a predicate that allows all events.
type Nop struct{}

func (p Nop) Create(_ event.CreateEvent) bool   { return true }
func (p Nop) Update(_ event.UpdateEvent) bool   { return true }
func (p Nop) Delete(_ event.DeleteEvent) bool   { return true }
func (p Nop) Generic(_ event.GenericEvent) bool { return true }
