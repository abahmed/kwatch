package pipeline

import (
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/metrics"
	"github.com/abahmed/kwatch/internal/pipeline/investigate"
)

// Investigation memory. An incident's evidence is kept after its
// announcement so a late result can join the next update; it is
// forgotten on resolve, or once it is this old and its incident closed.
const (
	evidenceMemory  = 30 * time.Minute
	maxEvidenceKept = 512
	// reinvestigateAfter is how old an incident's evidence may be when
	// a material change arrives before the logs are read again, so the
	// update quotes what the application says now, not at the first
	// message.
	reinvestigateAfter = 2 * time.Minute
)

// evidenceState is what the loop knows about one incident's
// investigation. Only the loop touches it.
type evidenceState struct {
	kind    string
	seq     int
	started time.Time
	running bool
	result  investigate.Result
	// sent marks the facts already delivered, so an update carries only
	// new ones; outputSent marks that the output went with a message.
	sent       map[evidenceKey]bool
	outputSent bool
}

// evidenceKey identifies one fact, so it is delivered once.
type evidenceKey struct {
	kind, subject, text string
}

// investigateOpened starts the investigation of every incident opened
// in this iteration, so its evidence is gathered while it settles.
func (a *announcer) investigateOpened(now time.Time) {
	opened := a.incidents.TakeOpened()
	if a.pool == nil {
		return
	}
	for _, p := range opened {
		a.investigate(p, now)
	}
}

// investigate plans and submits an investigation of p. It reports false
// when no investigator applies or no worker slot is free.
func (a *announcer) investigate(p incident.Incident, now time.Time) bool {
	plan, ok := a.investigator.Plan(p)
	if !ok {
		return false
	}
	a.forgetOldEvidence(now)
	state := a.evidence[p.ID]
	if state == nil {
		state = &evidenceState{sent: map[evidenceKey]bool{}}
	}
	job := investigationJob{id: p.ID, seq: state.seq + 1, plan: plan}
	if !a.pool.submit(job) {
		a.stats.skipped.Add(1)
		metrics.DefaultRegistry().IncInvestigation("skipped")
		return false
	}
	state.kind, state.seq, state.started = plan.Kind, job.seq, now
	state.running = true
	a.evidence[p.ID] = state
	return true
}

// awaitsEvidence reports whether decision d should wait for an
// investigation. One started at open is waited for; an incident whose
// root kind changed since, or that was never investigated, is
// investigated now. A material change whose evidence is older than
// reinvestigateAfter is investigated again, so the update carries the
// current output.
func (a *announcer) awaitsEvidence(d incident.Decision, now time.Time) bool {
	if a.pool == nil {
		return false
	}
	plan, ok := a.investigator.Plan(d.Incident)
	if !ok {
		return false
	}
	state := a.evidence[d.Incident.ID]
	if state != nil && state.kind == plan.Kind {
		if state.running {
			return true
		}
		if d.Action != incident.Update ||
			now.Sub(state.started) < reinvestigateAfter {
			return false
		}
	}
	return a.investigate(d.Incident, now)
}

// storeResult keeps a finished investigation's result. It reports false
// for a result an investigation started later replaced.
func (a *announcer) storeResult(r investigationResult) bool {
	state := a.evidence[r.id]
	if state == nil || state.seq != r.seq {
		return false
	}
	state.running = false
	state.result = r.result
	return true
}

// withEvidence attaches what investigation found to d. An announcement
// gets everything; an update only facts nobody has heard yet, and the
// output only with them, so evidence alone never makes news.
func (a *announcer) withEvidence(d incident.Decision) incident.Decision {
	state := a.evidence[d.Incident.ID]
	if state == nil || (d.Action != incident.Announce &&
		d.Action != incident.Update) {
		return d
	}
	var fresh []incident.Fact
	for _, ev := range state.result.Evidence {
		key := evidenceKey{kind: ev.Kind, subject: ev.Subject, text: ev.Text}
		if !state.sent[key] {
			state.sent[key] = true
			fresh = append(fresh, ev)
		}
	}
	if d.Action == incident.Announce || len(fresh) > 0 {
		d.Facts.Evidence = fresh
		if !state.outputSent {
			state.outputSent = len(state.result.Output) > 0
			d.Facts.Output = state.result.Output
		}
	}
	return d
}

// forgetEvidence drops the evidence of a resolved incident.
func (a *announcer) forgetEvidence(d incident.Decision) {
	if d.Action == incident.Resolve {
		delete(a.evidence, d.Incident.ID)
	}
}

// forgetOldEvidence drops evidence older than evidenceMemory unless its
// incident is still open, and the oldest beyond maxEvidenceKept, except
// investigations still running: their slot in the pool is still counted.
// An open incident keeps its evidence: forgetting what was already said
// would make its next update quote every fact again as new. The cap still
// bounds the memory.
func (a *announcer) forgetOldEvidence(now time.Time) {
	for id, state := range a.evidence {
		if !state.running && now.Sub(state.started) > evidenceMemory &&
			a.incidents.Closed([]string{id}) {
			delete(a.evidence, id)
		}
	}
	for len(a.evidence) >= maxEvidenceKept {
		oldest := ""
		for id, state := range a.evidence {
			if !state.running && (oldest == "" ||
				state.started.Before(a.evidence[oldest].started)) {
				oldest = id
			}
		}
		if oldest == "" {
			return
		}
		delete(a.evidence, oldest)
	}
}
