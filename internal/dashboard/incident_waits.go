package dashboard

import "time"

// The incident manager also answers two questions the incident list
// cannot: which incidents kwatch cannot verify now, and how many wait
// for the scope's verdict. A reader without them answers neither.

// unverifiedReader says why kwatch cannot verify an incident now.
type unverifiedReader interface {
	Unverified(id string, now time.Time) string
}

// scopeWaitingReader counts the incidents waiting for the scope.
type scopeWaitingReader interface {
	ScopeWaiting() int
}

// addUnverified adds to d why kwatch cannot verify incident id now.
func (s *Server) addUnverified(d *IncidentDetail, id string, now time.Time) {
	r, ok := s.incidentReader().(unverifiedReader)
	if !ok {
		return
	}
	text := r.Unverified(id, now)
	if text == "" {
		return
	}
	if d.Why == nil {
		d.Why = &WhyDetail{}
	}
	d.Why.UnverifiedReason = text
}

// scopeWaiting counts the incidents whose announcement waits for their
// namespace's labels; 0 without a reader that knows.
func (s *Server) scopeWaiting() int {
	if r, ok := s.incidentReader().(scopeWaitingReader); ok {
		return r.ScopeWaiting()
	}
	return 0
}

// incidentReader is the incident source; nil without one.
func (s *Server) incidentReader() incidentReader {
	if r := s.sources.incidents.Load(); r != nil {
		return *r
	}
	return nil
}
