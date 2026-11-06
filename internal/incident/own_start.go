package incident

import (
	"regexp"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// OwnStartWindow is how long after kwatch starts its own pods may wait
// for their volume without anyone being told. A replacement pod of a
// single-replica kwatch with a ReadWriteOnce volume waits for the
// volume to leave the old pod's node: a minute or a few, every
// restart, and never news. A wait that outlasts the window alerts as
// usual.
const OwnStartWindow = 10 * time.Minute

// levelOwnStart is the level code of such a wait (level.go).
const levelOwnStart = "own-start"

// ownStartWaits are the findings of a pod waiting for its volume.
var ownStartWaits = map[string]bool{
	reasons.VolumeAttachWaiting: true, reasons.VolumeMultiAttach: true,
}

// ownStartConsequences are what such a wait does to the pod and its
// workload: not ready, not available. They come with the wait and say
// nothing of their own.
var ownStartConsequences = map[string]bool{
	reasons.ContainersNotReady:    true,
	reasons.DeploymentUnavailable: true,
}

// noteStart remembers when this manager first ran: kwatch's start.
func (m *Manager) noteStart(now time.Time) {
	if m.started.IsZero() {
		m.started = now
	}
}

// generatedSuffix is what a controller appends to a workload's name: a
// ReplicaSet's template hash, a pod's random suffix.
var generatedSuffix = regexp.MustCompile(`^[a-z0-9]{4,10}$`)

// OwnWorkloadOf is the Deployment name of a pod named by its
// Deployment's ReplicaSet ("kwatch-5569b48575-bmfbl" is "kwatch"), or ""
// when the name does not look like one.
func OwnWorkloadOf(pod string) string {
	parts := strings.Split(pod, "-")
	if len(parts) < 3 || !generatedSuffix.MatchString(parts[len(parts)-1]) ||
		!generatedSuffix.MatchString(parts[len(parts)-2]) {
		return ""
	}
	return strings.Join(parts[:len(parts)-2], "-")
}

// own reports an object of kwatch's own workload: its Deployment, or an
// object whose owner chain (a pod's ReplicaSet, the ReplicaSet's
// Deployment) leads to it. The chain is what Kubernetes records, so a
// neighbour whose name only starts the same ("kwatch-db") is not own.
func (m *Manager) own(id inventory.EntityID) bool {
	w := m.cfg.OwnWorkload
	if w == "" || id.Namespace != m.cfg.OwnNamespace {
		return false
	}
	for range 4 { // the object and up to 3 owners
		if id.Kind == kube.KindDeployment && id.Name == w {
			return true
		}
		if m.model == nil {
			return false
		}
		owners := m.model.Related(id, inventory.OwnedBy,
			inventory.Outgoing)
		if len(owners) == 0 {
			return false
		}
		id = owners[0]
	}
	return false
}

// ownStartWait reports an incident in kwatch's own namespace whose
// failing members, all of kwatch's own workload, only wait for a volume
// (or are its consequences: not ready, not available), while kwatch
// started less
// than OwnStartWindow ago.
func (m *Manager) ownStartWait(p *Incident, now time.Time) bool {
	ns := m.cfg.OwnNamespace
	if ns == "" || p.Root.Namespace != ns || m.started.IsZero() ||
		!m.own(p.Root) ||
		!now.Before(m.started.Add(OwnStartWindow)) {
		return false
	}
	found := false
	for _, s := range p.Members {
		if s.Advisory {
			continue
		}
		if s.Entity.Namespace != ns || !m.own(s.Entity) {
			return false
		}
		if ownStartConsequences[s.Reason] {
			continue
		}
		if !ownStartWaits[s.Reason] {
			return false
		}
		found = true
	}
	return found
}

// quietOwnStart keeps such a wait silent, and gives an incident it kept
// silent its usual tier once the wait is no longer one.
func (m *Manager) quietOwnStart(p *Incident, now time.Time) {
	switch {
	case m.ownStartWait(p, now):
		p.Tier = Silent
		p.noteLevel(levelOwnStart)
	case p.Journal.Level == levelOwnStart && p.Tier == Silent:
		base, why := tierWhy(p)
		computed := m.override.apply(p, base)
		p.Tier = computed
		p.noteLevel(m.levelFor(p, base, why, computed))
	}
}
