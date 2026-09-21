package app

import (
	"encoding/json"
	"sync"
	"time"
)

type adoptionTelemetryStatus struct {
	mu                 sync.RWMutex
	state              string
	enabled            bool
	lastAttempt        time.Time
	lastSuccess        time.Time
	nextAttempt        time.Time
	consecutiveFailure int
	failureReason      string
}

type adoptionTelemetryStatusJSON struct {
	State              string    `json:"state"`
	Enabled            bool      `json:"enabled"`
	LastAttempt        time.Time `json:"lastAttempt,omitempty"`
	LastSuccess        time.Time `json:"lastSuccess,omitempty"`
	NextAttempt        time.Time `json:"nextAttempt,omitempty"`
	ConsecutiveFailure int       `json:"consecutiveFailures,omitempty"`
	FailureReason      string    `json:"failureReason,omitempty"`
}

func newAdoptionTelemetryStatus() *adoptionTelemetryStatus {
	return &adoptionTelemetryStatus{state: "waiting"}
}

func (s *adoptionTelemetryStatus) configure(
	enabled bool, state, reason string,
) {
	s.mu.Lock()
	s.enabled = enabled
	s.state = state
	s.failureReason = reason
	s.mu.Unlock()
}

func (s *adoptionTelemetryStatus) attempt(now time.Time) {
	s.mu.Lock()
	s.state = "sending"
	s.lastAttempt = now
	s.mu.Unlock()
}

func (s *adoptionTelemetryStatus) success(now, next time.Time) {
	s.mu.Lock()
	s.state = "waiting"
	s.lastSuccess = now
	s.nextAttempt = next
	s.consecutiveFailure = 0
	s.failureReason = ""
	s.mu.Unlock()
}

func (s *adoptionTelemetryStatus) failure(reason string, next time.Time) {
	s.mu.Lock()
	s.state = "retrying"
	s.failureReason = reason
	s.nextAttempt = next
	s.consecutiveFailure++
	s.mu.Unlock()
}

func (s *adoptionTelemetryStatus) waiting(next time.Time) {
	s.mu.Lock()
	s.state = "waiting"
	s.nextAttempt = next
	s.failureReason = ""
	s.mu.Unlock()
}

// StatusJSON implements health.StatusProvider without exposing cluster
// identity, endpoint, request data, or raw error strings.
func (s *adoptionTelemetryStatus) StatusJSON() ([]byte, error) {
	s.mu.RLock()
	status := adoptionTelemetryStatusJSON{
		State:              s.state,
		Enabled:            s.enabled,
		LastAttempt:        s.lastAttempt,
		LastSuccess:        s.lastSuccess,
		NextAttempt:        s.nextAttempt,
		ConsecutiveFailure: s.consecutiveFailure,
		FailureReason:      s.failureReason,
	}
	s.mu.RUnlock()
	return json.Marshal(status)
}
