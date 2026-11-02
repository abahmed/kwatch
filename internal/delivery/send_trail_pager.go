package delivery

import "time"

// Pager alert states the dashboard shows for one paging provider.
const (
	// PagerOpen: the pager accepted a page and no resolve closed it.
	PagerOpen = "open"
	// PagerClosed: the pager accepted the resolve after the page.
	PagerClosed = "closed"
	// PagerFailed: the pager refused the page for good.
	PagerFailed = "failed"
	// PagerPending: the page is still being retried.
	PagerPending = "pending"
)

// PagerState is what one pager holds for an incident: "Paged at T1,
// closed at T2", "still open on the pager" or "page failed".
type PagerState struct {
	Provider string `json:"provider"`
	State    string `json:"state"`
	// PagedAt is when the pager accepted the page; zero when it never
	// did. ClosedAt is when it accepted the resolve; zero while open.
	PagedAt  time.Time `json:"pagedAt"`
	ClosedAt time.Time `json:"closedAt"`
	// Reason is the failure reason of the last refused send since the
	// page was accepted.
	Reason string `json:"reason,omitempty"`
}

// PagerStates reads the pager state of each paging provider from an
// incident's trail, oldest first in the order the pagers were first
// tried. The trail keeps the first accepted attempt, so the time of the
// page survives the attempt bound.
func PagerStates(attempts []SendAttempt) []PagerState {
	var out []PagerState
	index := map[string]int{}
	for _, a := range attempts {
		if !a.Pager || a.Provider == "" {
			continue
		}
		i, seen := index[a.Provider]
		if !seen {
			i = len(out)
			index[a.Provider] = i
			out = append(out, PagerState{Provider: a.Provider,
				State: PagerPending})
		}
		out[i] = out[i].apply(a)
	}
	return out
}

// apply moves the state on by one attempt.
func (s PagerState) apply(a SendAttempt) PagerState {
	if a.Reason != "" {
		s.Reason = a.Reason
	}
	switch {
	case a.Kind == "resolve" && a.Result == resultOK:
		if !s.PagedAt.IsZero() {
			s.State, s.ClosedAt = PagerClosed, a.At
		}
	case a.Kind == "resolve":
		// A resolve that did not land leaves the alert as it was.
	case a.Result == resultOK:
		if s.State != PagerOpen {
			s.State, s.PagedAt, s.ClosedAt = PagerOpen, a.At, time.Time{}
		}
		s.Reason = ""
	case a.Result == "failed" && s.State != PagerOpen:
		s.State = PagerFailed
	}
	return s
}

// PagerStatus is PagerStates of the incident with id key.
func (m *Manager) PagerStatus(key string) []PagerState {
	return PagerStates(m.SendTrail(key))
}
