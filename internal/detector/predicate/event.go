package predicate

import (
	"github.com/abahmed/kwatch/internal/detector"
)

// event handles DELETED events
type Event struct{}

func NewEvent() *Event {
	return &Event{}
}

func (e *Event) Name() string {
	return "EventPredicate"
}

func (e *Event) Filter(input *detector.Input) bool {
	// Handle DELETED events - clean up memory
	if input.EventType == "DELETED" && input.Pod != nil {
		// This will be handled at handler level
		return false // don't filter, let it through
	}

	return false
}
