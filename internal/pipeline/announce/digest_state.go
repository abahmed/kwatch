package announce

import (
	"slices"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
)

// CarrierDropped is the carrier the audit log names for a decision the
// digest took in and then left out: an update with nothing new to list,
// or the resolve of a blip nobody was told about. Nobody hears it.
const CarrierDropped = "dropped"

// maxListed bounds how many listed incidents are remembered.
const maxListed = 500

// DigestLine names one pending digest entry by its incident. The text of
// the line is written again from the incident when the digest is sent.
type DigestLine struct {
	ID     string
	Reason incident.Reason `json:",omitempty"`
}

// DigestState is what the pending digest holds that no incident record
// keeps, persisted with the startup marker so a restart before the digest
// goes out loses nothing. The announcements waiting in the digest are not
// here: they are saved as held on their incidents and announced again.
type DigestState struct {
	// Since is when the first pending entry arrived.
	Since time.Time `json:",omitempty"`
	// Told are the pending lines of incidents people already heard of:
	// reminders and "failing again". They are not held on the incident.
	Told []DigestLine `json:",omitempty"`
	// Resolved are the pending resolves of incidents an earlier digest
	// listed.
	Resolved []DigestLine `json:",omitempty"`
	// Listed are the incidents a digest listed and that have not spoken
	// since: their first own message must introduce them in full.
	Listed []string `json:",omitempty"`
}

func (s DigestState) clone() DigestState {
	s.Told = slices.Clone(s.Told)
	s.Resolved = slices.Clone(s.Resolved)
	s.Listed = slices.Clone(s.Listed)
	return s
}

func (s DigestState) equal(o DigestState) bool {
	return s.Since.Equal(o.Since) && slices.Equal(s.Told, o.Told) &&
		slices.Equal(s.Resolved, o.Resolved) &&
		slices.Equal(s.Listed, o.Listed)
}

func linesOf(decisions []incident.Decision, told func(
	incident.Decision) bool) []DigestLine {
	var out []DigestLine
	for _, d := range decisions {
		if told(d) {
			out = append(out, DigestLine{ID: d.Incident.ID, Reason: d.Reason})
		}
	}
	return out
}

// pendingState is the digest state to persist now.
func (c *Collector) pendingState() DigestState {
	g := &c.Low
	state := DigestState{Since: g.Since, Listed: c.digested}
	state.Told = linesOf(g.Opened, func(d incident.Decision) bool {
		return d.Reason == incident.ReasonReminder ||
			d.Reason == incident.ReasonFailingAgain
	})
	state.Resolved = linesOf(g.Resolved, func(incident.Decision) bool {
		return true
	})
	return state
}

// saveDigest hands the writer the digest state when it changed.
func (c *Collector) saveDigest() {
	state := c.pendingState()
	if state.equal(c.Startup.Summary.Digest) {
		return
	}
	c.Startup.Summary.Digest = state.clone()
	c.env.SaveStartup(c.Startup.Summary)
}

// RestoreDigest rebuilds the pending digest of the previous run from the
// saved state and the restored incidents. A line whose incident is gone
// is left out.
func (c *Collector) RestoreDigest() {
	state := c.Startup.Summary.Digest
	c.digested = slices.Clone(state.Listed)
	if len(state.Told)+len(state.Resolved) == 0 {
		return
	}
	known := map[string]incident.Incident{}
	for _, p := range c.env.Incidents.Incidents() {
		known[p.ID] = p
	}
	for _, line := range state.Told {
		if p, ok := known[line.ID]; ok {
			c.Low.Opened = append(c.Low.Opened, incident.Decision{
				Action: incident.Update, Incident: p, Reason: line.Reason})
		}
	}
	for _, line := range state.Resolved {
		if p, ok := known[line.ID]; ok {
			c.Low.Resolved = append(c.Low.Resolved, incident.Decision{
				Action: incident.Resolve, Incident: p, Reason: "digest"})
		}
	}
	if len(c.Low.Opened)+len(c.Low.Resolved) > 0 {
		c.Low.Since = state.Since
	}
}

// markListed remembers that a digest listed incident id.
func (c *Collector) markListed(id string) {
	if slices.Contains(c.digested, id) {
		return
	}
	c.digested = append(c.digested, id)
	if over := len(c.digested) - maxListed; over > 0 {
		c.digested = slices.Clone(c.digested[over:])
	}
}

// takeListed reports whether a digest listed incident id and it has not
// spoken since, and forgets it: from now on it has spoken.
func (c *Collector) takeListed(id string) bool {
	i := slices.Index(c.digested, id)
	if i < 0 {
		return false
	}
	c.digested = slices.Delete(slices.Clone(c.digested), i, i+1)
	return true
}
