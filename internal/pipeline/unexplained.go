package pipeline

import (
	"encoding/json"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/metrics"
	"github.com/abahmed/kwatch/internal/status"
	"github.com/abahmed/kwatch/internal/unexplained"
)

// Bounds and pacing of the unexplained counts.
const (
	// maxOrigins bounds the Event objects whose last count is
	// remembered, so a repeated Event adds only what is new.
	maxOrigins = 4096
	// unexplainedSaveEvery paces writes: events can arrive in bursts.
	unexplainedSaveEvery = 30 * time.Second
	// unexplainedTop is how many reasons /status lists.
	unexplainedTop = 5
)

// unexplainedStore is the part of a store that keeps the unexplained
// counts. It is optional so test stores need not implement it.
type unexplainedStore interface {
	LoadUnexplained() (json.RawMessage, bool, error)
	SaveUnexplained(json.RawMessage) error
}

// unexplainedTracker counts what kwatch saw but could not explain: every
// occurrence of a Warning event reason it has no entry for, and every
// incident whose cause stayed unclear. It only counts: nothing it learns
// reaches detection, the tiers or the messages. Its methods run on the
// decision loop.
type unexplainedTracker struct {
	counts *unexplained.Counts
	// floor is the newest event the previous run counted; an event
	// listed again after a restart is not counted twice.
	floor time.Time
	// last is the count of each Event object already counted; order
	// lists the objects oldest first.
	last  map[string]int
	order []string
	now   func() time.Time
	// save hands the encoded counts to the history writer; nil without a
	// store.
	save      func(json.RawMessage)
	dirty     bool
	lastSaved time.Time
}

func newUnexplainedTracker(now func() time.Time) *unexplainedTracker {
	return &unexplainedTracker{counts: unexplained.New(),
		last: map[string]int{}, now: now}
}

// restore loads the counts of the previous run.
func (u *unexplainedTracker) restore(store IncidentStore) {
	saved, ok := store.(unexplainedStore)
	if !ok {
		return
	}
	raw, found, err := saved.LoadUnexplained()
	if err != nil || !found {
		if err != nil {
			klog.ErrorS(err, "pipeline: restore unexplained counts; "+
				"starting empty", "component", "pipeline",
				"operation", "restore")
		}
		return
	}
	restored := unexplained.New()
	if json.Unmarshal(raw, restored) == nil {
		u.counts = restored
		u.floor = restored.Latest()
	}
}

// observeNote counts the occurrences of a Warning event whose reason the
// registry has no entry for. An Event object that repeats adds only the
// occurrences it gained.
func (u *unexplainedTracker) observeNote(o inventory.Observation) {
	n := o.Note
	if !n.Warning || n.Reason == "" || detection.KnownReason(n.Reason) {
		return
	}
	added := max(n.Count, 1)
	if n.Origin != "" {
		before, seen := u.last[n.Origin]
		u.remember(n.Origin, max(n.Count, 1))
		switch {
		case seen:
			added -= before
		case !u.floor.IsZero() && !n.At.After(u.floor):
			// Listed again after a restart: counted by the last run.
			return
		case !u.floor.IsZero():
			// Seen before the restart too; only one occurrence is
			// known to be new.
			added = 1
		}
	}
	if added <= 0 {
		return
	}
	u.counts.Record(unexplained.KindEvent,
		string(detection.ModeUnusualEvent), n.Reason, n.Message, added, n.At)
	metrics.DefaultRegistry().AddUnexplained(unexplained.KindEvent, added)
	u.touch()
}

// remember keeps the last count of an Event object, forgetting the
// oldest objects past the bound.
func (u *unexplainedTracker) remember(origin string, count int) {
	if _, known := u.last[origin]; !known {
		u.order = append(u.order, origin)
	}
	u.last[origin] = count
	if over := len(u.order) - maxOrigins; over > 0 {
		for _, old := range u.order[:over] {
			delete(u.last, old)
		}
		u.order = append(u.order[:0:0], u.order[over:]...)
	}
}

// observeDecisions counts each incident whose cause is not clear once,
// at the first decision that says so.
func (u *unexplainedTracker) observeDecisions(
	now time.Time, decisions []incident.Decision,
) {
	for _, d := range decisions {
		inc := d.Incident
		if !inc.CauseUnclear || inc.Cause != nil {
			continue
		}
		mode, reason, sample := unclearEvidence(inc)
		if u.counts.RecordIncident(inc.ID, mode, reason, sample, now) {
			metrics.DefaultRegistry().AddUnexplained(
				unexplained.KindIncident, 1)
			u.touch()
		}
	}
}

// unclearEvidence names the incident by its worst member: the failure
// mode, the reason as Kubernetes or the detector worded it, and the first
// quoted evidence line (or the summary when it quotes nothing).
func unclearEvidence(inc incident.Incident) (mode, reason, sample string) {
	var worst detection.Finding
	for _, f := range inc.Members {
		f = detection.Classify(f)
		if worst.Reason == "" || f.Health > worst.Health ||
			(f.Health == worst.Health && f.Severity > worst.Severity) ||
			(f.Health == worst.Health && f.Severity == worst.Severity &&
				f.Reason < worst.Reason) {
			worst = f
		}
	}
	if worst.Reason == "" {
		return "unknown", "unknown", ""
	}
	sample = worst.Summary
	if len(worst.Evidence) > 0 {
		sample = worst.Evidence[0].Value
	}
	return string(worst.Mode), worst.Reason, sample
}

// touch marks the counts as changed and saves them when the pace allows.
func (u *unexplainedTracker) touch() {
	u.dirty = true
	now := u.now()
	if now.IsZero() || now.Sub(u.lastSaved) >= unexplainedSaveEvery {
		u.flush()
	}
}

// flush hands changed counts to the writer.
func (u *unexplainedTracker) flush() {
	if !u.dirty || u.save == nil {
		return
	}
	u.dirty = false
	u.lastSaved = u.now()
	if raw, err := json.Marshal(u.counts); err == nil {
		u.save(raw)
	}
}

// report is the unexplained counts as /status shows them.
func (u *unexplainedTracker) report(now time.Time) status.Unexplained {
	s := u.counts.Summary(now, unexplainedTop)
	out := status.Unexplained{Events: s.Events, Incidents: s.Incidents}
	for _, r := range s.Top {
		out.Top = append(out.Top, status.UnexplainedRow{
			Kind: r.Kind, Mode: r.Mode, Reason: r.Reason,
			Count: r.Count, Sample: lastSample(r.Samples),
		})
	}
	return out
}

func lastSample(samples []string) string {
	if len(samples) == 0 {
		return ""
	}
	return samples[len(samples)-1]
}
