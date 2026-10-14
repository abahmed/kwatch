package upstream

// Event is one reason a Kubernetes component puts on an Event. The
// list itself, Events, is generated (events_generated.go).
type Event struct {
	Reason string
	// Warning is true for an event the component records as type
	// Warning. Normal events are never ingested by kwatch.
	Warning bool
}
