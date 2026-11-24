package incident

import (
	"sort"
	"strconv"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// Grouping bounds: the groups listed and the members named per group.
const (
	MaxGroups       = 10
	MaxGroupMembers = 5
)

// What ties the members of a grouped incident together (Grouping.Basis).
const (
	GroupedLeftover   = "leftover"
	GroupedNode       = "node"
	GroupedPlace      = "place"
	GroupedNamespace  = "namespace"
	GroupedError      = "error"
	GroupedImage      = "image"
	GroupedRelease    = "release"
	GroupedDependency = "dependency"
	GroupedChain      = "chain"
	GroupedCause      = "cause"
)

// Grouping says why an incident holds several workloads or objects:
// what they share, from the cause and the members the incident already
// holds. Text is one plain sentence ("Grouped because all 12 pods run
// on node n1, which is not ready."), Basis what it rests on, and
// Groups the members by the workload (or object) they belong to.
type Grouping struct {
	Text   string
	Basis  string
	Groups []MemberGroup `json:",omitempty"`
	// More counts the groups left out past MaxGroups.
	More int `json:",omitempty"`
}

// MemberGroup is one workload, or one object that has no owner, with
// its failing members (pods, containers or the object itself).
type MemberGroup struct {
	Owner   inventory.EntityID
	Members []inventory.EntityID
	// More counts the members left out past MaxGroupMembers.
	More int `json:",omitempty"`
}

// memberUnits are an incident's failing members by the workload or
// object they belong to, and how many distinct pods they hold.
type memberUnits struct {
	groups map[inventory.EntityID][]inventory.EntityID
	pods   int
}

// GroupingOf says what ties p's members together, nil when p holds
// fewer than two workloads or objects besides its root, or when
// nothing kwatch holds names what they share. model may be nil: the
// grouping then rests on the cause and the members alone.
func GroupingOf(model inventory.Reader, p Incident) *Grouping {
	units := unitsOf(model, p)
	if len(units.groups) < 2 {
		return nil
	}
	basis, text := groupingText(model, p, units)
	if text == "" {
		return nil
	}
	out := &Grouping{Text: text, Basis: basis}
	out.Groups, out.More = units.listed()
	return out
}

// unitsOf groups p's non-advisory members by their top owner. The
// root, and the roots merged into p, are not members of a group.
func unitsOf(model inventory.Reader, p Incident) memberUnits {
	skip := map[inventory.EntityID]bool{p.Root: true}
	for _, id := range p.Merged {
		skip[id] = true
	}
	out := memberUnits{groups: map[inventory.EntityID][]inventory.EntityID{}}
	pods := map[inventory.EntityID]bool{}
	for _, f := range p.Members {
		if f.Advisory || skip[f.Entity] {
			continue
		}
		member, owner := f.Entity, f.Entity
		if model != nil {
			if pod, ok := rootcause.PodOf(model, f.Entity); ok {
				member, owner = pod, rootcause.TopOwner(model, pod)
				pods[pod] = true
			} else {
				owner = rootcause.TopOwner(model, f.Entity)
			}
		}
		if skip[owner] {
			continue
		}
		out.add(owner, member)
	}
	out.pods = len(pods)
	return out
}

// add files member under owner, once.
func (u memberUnits) add(owner, member inventory.EntityID) {
	for _, known := range u.groups[owner] {
		if known == member {
			return
		}
	}
	u.groups[owner] = append(u.groups[owner], member)
}

// workloads reports whether every group is a workload.
func (u memberUnits) workloads() bool {
	for owner := range u.groups {
		if !IsApplicationKind(owner.Kind) {
			return false
		}
	}
	return true
}

// word is the plural that names the groups: "workloads", the kind
// they all share ("nodes"), else "objects".
func (u memberUnits) word() string {
	if u.workloads() {
		return "workloads"
	}
	var kind inventory.Kind
	for owner := range u.groups {
		if kind != "" && owner.Kind != kind {
			return "objects"
		}
		kind = owner.Kind
	}
	return pluralOf(kindWord(kind))
}

// noun is "7 workloads", "2 nodes" or "7 objects".
func (u memberUnits) noun() string {
	return strconv.Itoa(len(u.groups)) + " " + u.word()
}

// listed are the groups, the most members first then by name, bounded.
func (u memberUnits) listed() ([]MemberGroup, int) {
	out := make([]MemberGroup, 0, len(u.groups))
	for owner, members := range u.groups {
		sorted := append([]inventory.EntityID(nil), members...)
		sort.Slice(sorted, func(i, j int) bool {
			return sorted[i].String() < sorted[j].String()
		})
		g := MemberGroup{Owner: owner, Members: sorted}
		if len(sorted) > MaxGroupMembers {
			g.Members, g.More = sorted[:MaxGroupMembers],
				len(sorted)-MaxGroupMembers
		}
		out = append(out, g)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if na, nb := len(a.Members)+a.More, len(b.Members)+b.More; na != nb {
			return na > nb
		}
		return a.Owner.String() < b.Owner.String()
	})
	if len(out) > MaxGroups {
		return out[:MaxGroups], len(out) - MaxGroups
	}
	return out, 0
}

// groupingText is the sentence for p's grouping and what it rests on,
// "" when nothing names what the members share.
func groupingText(
	model inventory.Reader, p Incident, u memberUnits,
) (string, string) {
	if members, ok := p.Leftovers(); ok && len(members) > 0 {
		if p.Root.Kind == kube.KindNamespace {
			return GroupedLeftover, "Grouped because all " + u.noun() +
				" in namespace " + p.Root.Name + " were scaled to 0 " +
				"while routes still send them traffic."
		}
		return GroupedLeftover, "Grouped because all " + u.noun() +
			" were left behind by " + objectWords(p.Root) + "."
	}
	switch p.Root.Kind {
	case kube.KindNode:
		return GroupedNode, "Grouped because " + podsOrUnits(u) +
			" run on node " + p.Root.Name + nodeState(model, p.Root) + "."
	case kube.KindZone, kube.KindNodePool:
		return GroupedPlace, "Grouped because " + podsOrUnits(u) +
			" run in " + objectWords(p.Root) + mergedNodes(p) + "."
	case kube.KindNamespace:
		return GroupedNamespace, "Grouped because " + u.noun() +
			" in namespace " + p.Root.Name + " failed together."
	}
	if p.Cause == nil {
		return "", ""
	}
	return causeGrouping(p, u)
}
