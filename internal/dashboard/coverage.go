package dashboard

import (
	"net/http"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// coverageSource builds the /api/coverage response: what kwatch
// watches, already bounded (see kube.Coverage).
type coverageSource func() kube.Coverage

// SetCoverage sets the coverage source; nil removes it (503).
func (s *Server) SetCoverage(src func() kube.Coverage) {
	if src == nil {
		s.sources.coverage.Store(nil)
		return
	}
	v := coverageSource(src)
	s.sources.coverage.Store(&v)
}

// coverageHandler serves GET /api/coverage: the namespaces and kinds
// kwatch watches, the kinds it cannot read and why, and when it synced.
func (s *Server) coverageHandler(w http.ResponseWriter, r *http.Request) {
	if !onlyGet(w, r) {
		return
	}
	src := s.sources.coverage.Load()
	if src == nil {
		unavailable(w)
		return
	}
	writeTagged(w, r, (*src)())
}
