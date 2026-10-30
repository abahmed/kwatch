package incident

import (
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// awaitingEvidence reports whether an incident without members must keep
// its state: during the restore grace detectors may not have re-raised
// its findings yet, and a root kind kwatch cannot observe says nothing
// about recovery.
func (m *Manager) awaitingEvidence(p *Incident, now time.Time) bool {
	p.heldForData = false
	if len(p.Members) > 0 {
		return false
	}
	if m.inGrace(p, now) {
		return true
	}
	if m.verifiable(p) {
		return false
	}
	p.heldForData = true
	p.noteHeld(now)
	return true
}

// heldNote says on the timeline why an incident with no failure left
// stays open.
const heldNote = "resolve held: kwatch cannot observe this object now"

// heldRecheck is how often an incident held for missing data looks again.
// Sight can come back without any finding changing, so nothing else would
// wake it.
const heldRecheck = time.Minute

// noteHeld adds heldNote once per hold, also across a restart: the
// timeline is persisted.
func (p *Incident) noteHeld(now time.Time) {
	if n := len(p.Timeline); n > 0 && p.Timeline[n-1].Text == heldNote {
		return
	}
	p.note(now, heldNote)
}

// inGrace reports whether p is a restored incident still inside the
// restore grace. Incidents opened after the restart have no stale
// model to wait for: their members come from this session's detectors.
// A node that is ready again has no findings to wait for either.
func (m *Manager) inGrace(p *Incident, now time.Time) bool {
	if !p.restored || !now.Before(m.restoreGrace) {
		return false
	}
	_, recovered := m.recoveredWhileDown(p)
	return !recovered
}

// holdRecovery reports whether a recovering incident without members
// must not resolve yet. During the restore grace the model is still being
// rebuilt, so missing members say nothing; the hold keeps running. A root
// kwatch cannot observe restarts the hold instead of resolving on missing
// data.
func (m *Manager) holdRecovery(p *Incident, now time.Time) bool {
	if !m.awaitingEvidence(p, now) {
		return false
	}
	if !m.verifiable(p) {
		p.RecoveringSince = now
	}
	return true
}

// afterGrace moves a deadline that falls inside the restore grace to the
// end of the grace, when recovery may first be decided.
func (m *Manager) afterGrace(
	p *Incident, at, now time.Time,
) time.Time {
	if m.inGrace(p, now) && at.Before(m.restoreGrace) {
		return m.restoreGrace
	}
	return at
}

// verifiable reports whether kwatch can observe what recovery of p is
// judged from: the root and, for a root that owns pods, the pods.
// A restored incident has no members until the pods are listed, so a
// synced Deployment over an unsynced Pod list proves nothing.
func (m *Manager) verifiable(p *Incident) bool {
	if !m.rootVerifiable(p.Root) {
		return false
	}
	return m.cfg.Verifiable == nil || !ownsPods(p.Root.Kind) ||
		m.cfg.Verifiable(kube.KindPod)
}

func (m *Manager) rootVerifiable(root inventory.EntityID) bool {
	if m.cfg.VerifiableObject != nil {
		return m.cfg.VerifiableObject(root)
	}
	return m.cfg.Verifiable == nil || m.cfg.Verifiable(root.Kind)
}

// ownsPods reports a root kind whose members are the pods behind it.
func ownsPods(kind inventory.Kind) bool {
	switch kind {
	case kube.KindDeployment, kube.KindStatefulSet, kube.KindDaemonSet,
		kube.KindReplicaSet, kube.KindJob, kube.KindCronJob,
		kube.KindService:
		return true
	}
	return false
}

// ExtendGrace moves the restore grace to until when that is later.
// The grace set at restore time starts before the sources synced, and
// findings gated by a sustain re-raise one sustain after the sync, so
// the engine extends it when the sync happens. Without a restored live
// incident there is no grace to extend.
func (m *Manager) ExtendGrace(until time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.restoreGrace.IsZero() && until.After(m.restoreGrace) {
		m.restoreGrace = until
	}
}
