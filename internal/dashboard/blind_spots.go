package dashboard

import (
	"cmp"
	"slices"
)

// componentSource is the optional parts of kwatch that are not working,
// by name, each with its health reason (health.ComponentErrors).
type componentSource func() map[string]string

// BlindSpot is one optional part of kwatch that does not work now, so
// kwatch sees or does less: Part is its health component name
// (kubelet-stats, rbac, license, ...) and Reason the health reason code
// (kubelet_unreachable, license_expired, ...). The page words both.
type BlindSpot struct {
	Part   string `json:"part"`
	Reason string `json:"reason,omitempty"`
}

// shownElsewhere are the parts the kwatch page already shows from
// their own fields: pod log access (self.podLogsForbidden) and the
// kinds kwatch cannot read (coverage).
var shownElsewhere = map[string]bool{"pod-logs": true, "coverage": true}

// SetComponentErrors sets the source of kwatch's failed optional parts;
// nil leaves blindSpots out of /api/status.
func (s *Server) SetComponentErrors(src func() map[string]string) {
	if src == nil {
		s.sources.components.Store(nil)
		return
	}
	v := componentSource(src)
	s.sources.components.Store(&v)
}

// blindSpots lists the parts of kwatch that are not working, sorted by
// part; nil without a source or when every part works.
func (s *Server) blindSpots() []BlindSpot {
	src := s.sources.components.Load()
	if src == nil {
		return nil
	}
	var out []BlindSpot
	for part, reason := range (*src)() {
		if !shownElsewhere[part] {
			out = append(out, BlindSpot{Part: part, Reason: reason})
		}
	}
	slices.SortFunc(out, func(a, b BlindSpot) int {
		return cmp.Compare(a.Part, b.Part)
	})
	return out
}
