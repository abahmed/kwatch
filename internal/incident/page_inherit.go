package incident

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
)

// A cause revision or a flip of the reasoning can split one outage over
// two incidents: the failure of the new incident is the story of a page
// that people already got. The new incident is announced, so the thread
// and the chat see it, but it does not page again within RepageWindow.

// inheritPage gives the new incident p the page of the incident of the
// same story that paged, as the history of a page that resolved just
// now. The page then follows the rule of a page that came back: it
// notifies and says it failed again.
func (m *Manager) inheritPage(
	now time.Time, p *Incident, s detection.Finding,
) {
	if p.State != Settling || len(p.Members) > 0 || !p.Announced.IsZero() {
		return
	}
	for _, id := range m.sortedIDs() {
		old := m.incidents[id]
		if old == p || !m.pagedStory(now, old, s) {
			continue
		}
		o := occurrenceOf(old)
		if old.State != Resolved {
			o.Resolved = now
		}
		p.History = appendHistory(p.History, o)
		return
	}
}

// pagedStory reports whether old paged for the story of s: it is live or
// ended less than RepageWindow ago, people got its page, and s is about
// its workload.
func (m *Manager) pagedStory(
	now time.Time, old *Incident, s detection.Finding,
) bool {
	if old.State == Resolved && now.Sub(old.Resolved) > RepageWindow {
		return false
	}
	return pagedOccurrence(old) && m.ofStory(old, s)
}
