package incident

import (
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// Reasons of the updates that follow a fix attempt.
const (
	// ReasonFixAttempt is an update because someone changed the root's
	// workload, or a ConfigMap or Secret it uses, while the incident was
	// open: a rollout started, a config was edited. People watching the
	// thread want to know that a fix is on its way.
	ReasonFixAttempt Reason = "fix attempt"
	// ReasonFixStillFailing is the one update that says the incident
	// still fails FixWatch after its fix attempt.
	ReasonFixStillFailing Reason = "fix attempt still failing"
)

// attemptKinds are the objects whose edit counts as an attempt to fix a
// failure: the workloads and the configuration their pods read.
var attemptKinds = map[inventory.Kind]bool{
	kube.KindDeployment:  true,
	kube.KindStatefulSet: true,
	kube.KindDaemonSet:   true,
	kube.KindConfigMap:   true,
	kube.KindSecret:      true,
}

// touchesAttemptKind reports whether one of ids could carry a fix
// attempt, so the manager looks at the change history only then.
func touchesAttemptKind(ids []inventory.EntityID) bool {
	for _, id := range ids {
		if attemptKinds[id.Kind] {
			return true
		}
	}
	return false
}

// findAttempt returns the newest fix attempt on p's root workload, or on
// a ConfigMap or Secret it uses, made after the incident was announced
// and after the attempt already reported. Scaling is no attempt: an
// autoscaler reacts to a failure, it does not fix it. Nil when none.
func findAttempt(
	model inventory.HistoryReader, p *Incident, now time.Time,
) *inventory.Change {
	if model == nil {
		return nil
	}
	since := p.Announced
	for _, known := range []*inventory.Change{p.Attempt, p.attemptSeen} {
		if known != nil && known.At.After(since) {
			since = known.At
		}
	}
	var latest *inventory.Change
	for _, set := range model.ChangeSets(p.Root, since) {
		for _, change := range set.Changes {
			if !isAttempt(change, since, now) {
				continue
			}
			if latest == nil || change.At.After(latest.At) {
				found := change
				latest = &found
			}
		}
	}
	return latest
}

// isAttempt reports a change after since that edits a workload's pod
// template or a configuration object's data.
func isAttempt(change inventory.Change, since, now time.Time) bool {
	if !attemptKinds[change.Entity.Kind] || !change.At.After(since) ||
		change.At.After(now) || change.Deleted || change.Created {
		return false
	}
	switch change.Classify() {
	case inventory.ClassRollout, inventory.ClassImage, inventory.ClassConfig:
		return true
	}
	return false
}

// attemptMessageBudget is how many messages (Incident.Revision counts
// them) an incident may already have sent and still get a fix-attempt
// update: the announcement, a few updates, the resolve. Past it the
// attempt is remembered, so the resolve can credit it, but not announced.
const attemptMessageBudget = 3

// observeAttempts records, for every open incident, a fix attempt that
// arrived with this Apply. It runs when a workload or configuration
// object changed, so the change history is read only then. Recording is
// separate from telling: an earlier step may end the tick, and the
// attempt waits in attemptSeen for a tick when none does.
func (m *Manager) observeAttempts(dirty []inventory.EntityID, now time.Time) {
	if !touchesAttemptKind(dirty) {
		return
	}
	for _, id := range m.liveIDs() {
		p := m.incidents[id]
		if p.State != Open {
			continue
		}
		if change := findAttempt(m.model, p, now); change != nil {
			p.attemptSeen = change
		}
	}
}

// attemptUpdate decides the update about a fix attempt, if one is due:
// first when the change was seen, once more when the incident still
// fails FixWatch later. A pending material change goes first, so the
// attempt never swallows news; the attempt stays recorded until then.
func (m *Manager) attemptUpdate(p *Incident, now time.Time) (Decision, bool) {
	if p.Tier < Notify || fingerprint(p) != p.Digest {
		return Decision{}, false
	}
	why := m.noteAttempt(p, now)
	switch {
	case why == "":
		return Decision{}, false
	case p.Revision > attemptMessageBudget:
		// Out of budget: remember it without a message.
		p.Digest = fingerprint(p)
		return Decision{}, false
	}
	return m.decide(p, Update, why), true
}

// noteAttempt moves a recorded fix attempt to Attempt, or notes that
// the last one still fails FixWatch later, and returns the reason to
// tell people. Empty when there is nothing new.
func (m *Manager) noteAttempt(p *Incident, now time.Time) Reason {
	if p.attemptSeen != nil {
		p.Attempt, p.attemptLate = p.attemptSeen, false
		p.attemptSeen = nil
		return ReasonFixAttempt
	}
	if p.Attempt != nil && !p.attemptLate &&
		!now.Before(p.Attempt.At.Add(FixWatch)) {
		p.attemptLate = true
		return ReasonFixStillFailing
	}
	return ""
}
