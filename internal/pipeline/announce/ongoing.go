package announce

import (
	"sort"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/notification/compose"
)

// OngoingEvery is how soon a digest-tier problem that keeps coming back
// is worth a digest of its own. Any digest that goes out anyway lists
// every ongoing problem; this only keeps one that nothing else reports
// from being forgotten for good.
const OngoingEvery = 6 * time.Hour

// ongoingCheckEvery is how often the open incidents are looked at for a
// problem that came back since its last digest.
const ongoingCheckEvery = 5 * time.Minute

// ongoingItem is an ongoing problem and the incident behind it.
type ongoingItem struct {
	p incident.Incident
	compose.Ongoing
}

// ongoingProblems are the digest-tier incidents an earlier digest listed
// that are still open and that the digest being written does not list
// already, oldest first.
func (c *Collector) ongoingProblems() []ongoingItem {
	var out []ongoingItem
	for _, p := range c.openListed() {
		if indexOfIncident(c.Low.Opened, p.ID) >= 0 ||
			indexOfIncident(c.Low.Resolved, p.ID) >= 0 {
			continue
		}
		reason := ongoingReason(p)
		out = append(out, ongoingItem{p: p, Ongoing: compose.Ongoing{
			Root: p.Root, Reason: reason, Since: p.Opened,
			Count: c.timesSeen(p, reason)}})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Since.Before(out[j].Since)
	})
	return out
}

// openListed are the incidents of the digest tier an earlier digest
// listed, or that fell to it from a thread, still failing, that people
// want to hear about.
func (c *Collector) openListed() []incident.Incident {
	var out []incident.Incident
	for _, p := range c.env.Incidents.Incidents() {
		open := p.State == incident.Open || p.State == incident.Flapping ||
			p.State == incident.Recovering
		if open && p.Ack == nil && p.Tier == incident.Digest &&
			(!p.DigestedAt.IsZero() || p.Delivery.Demoted()) &&
			(c.env.Scope == nil || c.env.Scope(p)) {
			out = append(out, p)
		}
	}
	return out
}

// ongoingReason names what keeps failing, as Kubernetes does for an
// event: the reason of the root's own finding, else of the most serious
// one.
func ongoingReason(p incident.Incident) string {
	return strings.TrimPrefix(findingReason(p), reasons.UnusualEventPrefix)
}

func findingReason(p incident.Incident) string {
	var best detection.Finding
	for _, f := range p.Members {
		if f.Advisory {
			continue
		}
		own, bestOwn := f.Entity == p.Root, best.Entity == p.Root
		if best.Reason == "" || (own && !bestOwn) ||
			(own == bestOwn && (f.Severity > best.Severity ||
				(f.Severity == best.Severity && f.Reason < best.Reason))) {
			best = f
		}
	}
	return best.Reason
}

// timesSeen counts the events of the problem's reason about its root
// since it opened, as Kubernetes counted them; zero when unknown.
func (c *Collector) timesSeen(p incident.Incident, reason string) int {
	if c.env.History == nil || reason == "" {
		return 0
	}
	count := 0
	for _, note := range c.env.History.Notes(p.Root, p.Opened) {
		if note.Reason == reason && note.Warning {
			count += max(note.Count, 1)
		}
	}
	return count
}

// noteOngoing opens the digest window when a listed problem came back
// since its last digest and none has been said for OngoingEvery: a
// problem that never goes away must not stay unmentioned for good.
func (c *Collector) noteOngoing(now time.Time) {
	if c.env.History == nil || now.Sub(c.ongoingChecked) < ongoingCheckEvery {
		return
	}
	c.ongoingChecked = now
	for _, p := range c.openListed() {
		count := c.timesSeen(p, ongoingReason(p))
		mark, known := c.ongoingMarks[p.ID]
		if !known {
			c.ongoingMarks[p.ID] = count
			continue
		}
		if count > mark && now.Sub(p.DigestedAt) >= OngoingEvery &&
			c.Low.Since.IsZero() {
			c.Low.Since = now
		}
	}
}

// markSeen remembers how often a problem a digest listed had been seen,
// so the next check can tell that it came back.
func (c *Collector) markSeen(p incident.Incident) {
	c.ongoingMarks[p.ID] = c.timesSeen(p, ongoingReason(p))
}
