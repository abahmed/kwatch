package incident

import (
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// Objects stuck on one finalizer were left behind by the controller
// that should remove it, and they come back together: after each
// restart of kwatch, each relist, each time their grace runs out. One
// placed by itself first, before the engine names that finalizer,
// would open an incident of its own for a few seconds and resolve as
// "cause revised", again and again. joinStuckPeers places it at once
// where the objects held by the same finalizer already are.

// joinStuckPeers places a low stuck deletion with the live incident
// that already holds an object stuck on one of its finalizers in its
// namespace. A placement of its own (no cause) joins that incident,
// a leftover group first; a placement at the finalizer itself joins
// only a leftover group, which says more: whose leftovers they are.
// The finding is marked a leftover when it joins a leftover group.
func (m *Manager) joinStuckPeers(
	s detection.Finding, where placement,
) (detection.Finding, placement) {
	if !isStuckDeletion(s) || !lowAlone(s) || !s.Leftover.IsZero() ||
		(where.cause != nil && where.root.Kind != explain.KindFinalizer) {
		return s, where
	}
	peer, group := m.stuckPeer(s)
	switch {
	case peer == nil || peer.Root == where.root:
		return s, where
	case group:
		s.Leftover = peer.Root
	case where.cause != nil:
		return s, where
	}
	return s, placement{root: peer.Root}
}

// stuckPeer is the live incident, a leftover group first, then the
// first by ID, that holds an object of s's namespace stuck on one of
// s's finalizers; group reports a leftover group.
func (m *Manager) stuckPeer(s detection.Finding) (*Incident, bool) {
	names := finalizerNames(s)
	if len(names) == 0 {
		return nil, false
	}
	var best *Incident
	bestGroup := false
	for _, p := range m.incidents {
		if p.State == Resolved || !holdsStuckPeer(p, s, names) {
			continue
		}
		_, group := p.Leftovers()
		if best == nil || (group && !bestGroup) ||
			(group == bestGroup && p.ID < best.ID) {
			best, bestGroup = p, group
		}
	}
	return best, bestGroup
}

// holdsStuckPeer reports a member of p, s itself included, stuck
// deleting in s's namespace on one of names: a member stays where it
// is.
func holdsStuckPeer(
	p *Incident, s detection.Finding, names []string,
) bool {
	for _, member := range p.Members {
		if !isStuckDeletion(member) ||
			member.Entity.Namespace != s.Entity.Namespace {
			continue
		}
		for _, name := range finalizerNames(member) {
			if containsText(names, name) {
				return true
			}
		}
	}
	return false
}

// isStuckDeletion reports a finding of an object whose deletion a
// finalizer holds.
func isStuckDeletion(f detection.Finding) bool {
	return f.Mode == detection.ModeStuckDeleting ||
		f.Reason == reasons.StuckDeleting
}

// finalizerNames are the finalizers a stuck deletion names.
func finalizerNames(f detection.Finding) []string {
	var out []string
	for _, e := range f.Evidence {
		if e.Label != detection.EvidenceFinalizers {
			continue
		}
		for _, name := range strings.Split(e.Value, ",") {
			if name = strings.TrimSpace(name); name != "" {
				out = append(out, name)
			}
		}
	}
	return out
}

func containsText(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
