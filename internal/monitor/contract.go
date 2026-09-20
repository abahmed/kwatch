package monitor

import "github.com/abahmed/kwatch/internal/model"

// ObservationSink is the narrow incident boundary available to a monitor.
// Monitors may report facts or resolve a subject, but they cannot deliver
// notifications or write persistence directly.
type ObservationSink interface {
	Process(*model.Observation) (*model.Incident, model.IncidentAction)
	Resolve(model.ObjectRef, string)
	ResolveObserved(*model.Observation)
}

// ReconciliationSink adds object-level recovery to the observation boundary.
// Families use it when several findings belong to one Kubernetes object.
type ReconciliationSink interface {
	ObservationSink
	Reconcile(model.ObjectRef, []*model.Observation)
	ReconcileGone(model.ObjectRef)
}
