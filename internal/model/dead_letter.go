package model

import "time"

// DeadLetterEntry is the diagnostic representation of a delivery that could
// not be completed after retry and fallback handling.
type DeadLetterEntry struct {
	Provider  string         `json:"provider"`
	Key       string         `json:"key"`
	Action    IncidentAction `json:"action"`
	Error     string         `json:"error"`
	Timestamp time.Time      `json:"timestamp"`
}
