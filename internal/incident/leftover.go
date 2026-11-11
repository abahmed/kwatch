package incident

import (
	"slices"
	"sort"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// Leftovers of a removed controller. An operator that was uninstalled
// or scaled to zero leaves objects behind: webhooks that call its gone
// Service, its expired certificates, its parked Deployments, objects
// stuck on its finalizers. Each is a finding of its own, and each
// alone would be an incident of its own. The detectors name the absent
// backend they share (detection.Finding.Leftover), and every such
// finding that is low on its own joins one incident rooted at it. A
// finding that blocks or breaks something keeps its own incident and
// tier: it never carries the mark.

// joinLeftover places a low leftover finding at the absent backend it
// was left behind by, once that backend left at least two objects
// behind: one alone keeps the incident and the words of its own
// object. A placement already at the backend keeps its cause.
func (m *Manager) joinLeftover(
	s detection.Finding, where placement,
) placement {
	if s.Leftover.IsZero() || !lowAlone(s) || where.root == s.Leftover ||
		(m.leftovers[s.Leftover] < minLeftovers && !m.inLiveGroup(s)) {
		return where
	}
	return placement{root: s.Leftover}
}

// inLiveGroup reports a leftover that is already in the live group of
// its backend. It stays there when the others cleared and it is the
// last one: moving it to an incident of its own would tell nothing new.
func (m *Manager) inLiveGroup(s detection.Finding) bool {
	p := m.incidents[m.byMember[s.Key()]]
	return p != nil && p.State != Resolved && p.Root == s.Leftover
}

// minLeftovers is how many objects make the leftovers of one backend a
// group of their own.
const minLeftovers = 2

// noteLeftovers counts, per absent backend, the objects the active
// findings say it left behind.
func (m *Manager) noteLeftovers(s explain.Snapshot) {
	clear(m.leftovers)
	for _, findings := range s.Findings {
		var counted []inventory.EntityID
		for _, f := range findings {
			if f.Leftover.IsZero() || !lowAlone(f) ||
				slices.Contains(counted, f.Leftover) {
				continue
			}
			counted = append(counted, f.Leftover)
			if m.leftovers == nil {
				m.leftovers = map[inventory.EntityID]int{}
			}
			m.leftovers[f.Leftover]++
		}
	}
}

// regroupLeftovers moves the leftovers that opened an incident of their
// own, before their backend left another object behind, to the group
// of that backend. So does a leftover that was left out as an old
// problem (old_problem.go) while it was the only one: the group is one
// Low incident, and it names every object.
func (m *Manager) regroupLeftovers(s explain.Snapshot) {
	for _, findings := range s.Findings {
		for _, f := range findings {
			if f.Leftover.IsZero() || !lowAlone(f) ||
				m.leftovers[f.Leftover] < minLeftovers {
				continue
			}
			id, ok := m.byMember[f.Key()]
			p := m.incidents[id]
			_, aged := m.aged[f.Key()]
			if aged || (ok && p != nil && p.Root != f.Leftover) {
				m.attach(s.Now, f, placement{root: f.Leftover})
			}
		}
	}
}

// handlerLeftover marks a low stuck deletion a leftover when the
// engine blames its finalizer's controller, and that controller's
// namespace already has an absent backend: the stuck objects of a
// removed operator usually live in other namespaces than the operator.
func handlerLeftover(
	s explain.Snapshot, f detection.Finding, where placement,
) detection.Finding {
	if !f.Leftover.IsZero() || f.Mode != detection.ModeStuckDeleting ||
		!lowAlone(f) || where.cause == nil || !IsWorkload(where.root.Kind) {
		return f
	}
	f.Leftover = leftoverIn(s, where.root.Namespace)
	return f
}

// leftoverIn is the absent backend that the active findings name for
// namespace, the first by name; zero when none does.
func leftoverIn(s explain.Snapshot, namespace string) inventory.EntityID {
	var found inventory.EntityID
	for _, findings := range s.Findings {
		for _, f := range findings {
			id := f.Leftover
			if id.IsZero() || id.Namespace != namespace ||
				(!found.IsZero() && id.Name >= found.Name) {
				continue
			}
			found = id
		}
	}
	return found
}

// LeftoverMember is one object a removed controller left behind.
type LeftoverMember struct {
	Entity  inventory.EntityID
	Reason  string
	Summary string
}

// Leftovers lists the objects of an incident that groups the leftovers
// of a removed controller, by object then reason; ok is false for any
// other incident. Such an incident is rooted at the absent backend and
// every failing member was left behind by it.
func (p Incident) Leftovers() (members []LeftoverMember, ok bool) {
	for _, s := range p.Members {
		if s.Advisory {
			continue
		}
		if s.Leftover != p.Root {
			return nil, false
		}
		members = append(members, LeftoverMember{Entity: s.Entity,
			Reason: s.Reason, Summary: s.Summary})
	}
	sort.Slice(members, func(i, j int) bool {
		a, b := members[i], members[j]
		if a.Entity != b.Entity {
			return a.Entity.String() < b.Entity.String()
		}
		return a.Reason < b.Reason
	})
	return members, len(members) > 0
}

// LeftoverObjects counts the distinct objects of a leftover incident;
// zero for any other incident.
func (p Incident) LeftoverObjects() int {
	members, ok := p.Leftovers()
	if !ok {
		return 0
	}
	seen := map[inventory.EntityID]bool{}
	for _, member := range members {
		seen[member.Entity] = true
	}
	return len(seen)
}
