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
	writeTagged(w, r, withTotals((*src)()))
}

// CoverageResponse is kube.Coverage with the full size of each kind
// list, which keeps at most kube.MaxCoverageNames entries: the page
// says "+N more" from KindTotals. The namespace lists have theirs in
// namespaces.watched and namespaces.excluded.
type CoverageResponse struct {
	kube.Coverage
	KindTotals KindTotals `json:"kindTotals"`
}

// KindTotals count every kind kwatch cannot read, has not read yet and
// does not watch, the ones the lists leave out included.
type KindTotals struct {
	Unreadable int `json:"unreadable"`
	Unsynced   int `json:"unsynced"`
	NotWatched int `json:"notWatched"`
}

// withTotals adds the list totals from the counts by reason, filed the
// way kube files each gap; a list never counts less than it holds.
func withTotals(c kube.Coverage) CoverageResponse {
	var t KindTotals
	for reason, n := range c.Kinds.Reasons {
		switch reason {
		case kube.ReasonPermissionDenied:
			t.Unreadable += n
		case kube.ReasonSyncPending, kube.ReasonSyncTimeout,
			kube.ReasonSyncFailed:
			t.Unsynced += n
		default:
			t.NotWatched += n
		}
	}
	t.Unreadable = max(t.Unreadable, len(c.Kinds.Unreadable))
	t.Unsynced = max(t.Unsynced, len(c.Kinds.Unsynced))
	t.NotWatched = max(t.NotWatched, len(c.Kinds.NotWatched))
	return CoverageResponse{Coverage: c, KindTotals: t}
}
