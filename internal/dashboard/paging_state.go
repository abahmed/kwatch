package dashboard

import (
	"time"

	"github.com/abahmed/kwatch/internal/incident"
)

// pagingState is what the incident reader may also say about paging:
// a scaling wave holding pages, and the non-production setting.
// *incident.Manager has both; a reader without them says neither.
type pagingState interface {
	Scaling(now time.Time) incident.ScalingState
	NonProduction() bool
}

// ScalingHold says pages are held because the cluster is scaling:
// Since is when the nodes began to come and go, ExpectedEnd when the
// hold ends if no node changes again (the wave over, plus a grace).
type ScalingHold struct {
	Reason      string    `json:"reason"`
	Since       time.Time `json:"since"`
	ExpectedEnd time.Time `json:"expectedEnd"`
}

// pagingOf is the paging state of the incident reader, if it has one.
func (s *Server) pagingOf() (pagingState, bool) {
	reader := s.sources.incidents.Load()
	if reader == nil {
		return nil, false
	}
	p, ok := (*reader).(pagingState)
	return p, ok
}

// scalingHold is the scaling hold at now; nil when pages are not held.
// A non-production cluster pages nobody, so it has no page to hold.
func (s *Server) scalingHold(now time.Time) *ScalingHold {
	p, ok := s.pagingOf()
	if !ok || p.NonProduction() {
		return nil
	}
	st := p.Scaling(now)
	if !st.Held {
		return nil
	}
	return &ScalingHold{Reason: incident.ScalingHeldText,
		Since: st.Since, ExpectedEnd: st.Expected}
}

// nonProduction reports the non-production setting of the cluster.
func (s *Server) nonProduction() bool {
	p, ok := s.pagingOf()
	return ok && p.NonProduction()
}
