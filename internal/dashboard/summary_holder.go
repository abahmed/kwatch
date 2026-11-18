package dashboard

import (
	"fmt"
	"sort"
	"strings"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// holderClass is which objects can have a symptom: "What's wrong?"
// never says a Secret exits, a Node fails a readiness check or an
// APIService scales.
type holderClass uint8

const (
	// holdsAny is any object the API serves.
	holdsAny holderClass = iota
	// holdsCode is what runs code: pods, containers, their controllers
	// and custom kinds, which may run code too.
	holdsCode
	// holdsNode is a node.
	holdsNode
	// holdsTraffic is a Service.
	holdsTraffic
	// holdsSender is an object that sends traffic on: a Service, an
	// Ingress, a webhook configuration, a custom route.
	holdsSender
	// holdsScaling is an autoscaler, or a custom kind that may be one.
	holdsScaling
)

// probeFamily is the family a failing probe stands for in holderOf.
const probeFamily = "Probe"

// familyHolders are the classes of the families that not every object
// can have; any other family is holdsAny.
var familyHolders = map[string]holderClass{
	"CrashLoop": holdsCode, "Restarting": holdsCode, "Error": holdsCode,
	"Killed": holdsCode, "OOMKilled": holdsCode, "ImagePull": holdsCode,
	"CreateError": holdsCode, "InitError": holdsCode,
	"InitWaiting": holdsCode, "Hook": holdsCode,
	"Unschedulable": holdsCode, "Pending": holdsCode,
	"NotScheduling": holdsCode, "SchedulingGated": holdsCode,
	"Evicted": holdsCode, "JobFailed": holdsCode,
	"DeadlineExceeded": holdsCode, "RolloutStuck": holdsCode,
	"Unavailable": holdsCode, "ReplicaFailure": holdsCode,
	"Missing": holdsCode, "Suspended": holdsCode,
	"InvalidSchedule": holdsCode, probeFamily: holdsCode,
	"MemoryPressure": holdsNode, "DiskPressure": holdsNode,
	"PIDPressure": holdsNode, "Draining": holdsNode, "Node": holdsNode,
	"NetworkUnavailable": holdsNode, "Unreachable": holdsNode,
	"NoEndpoints": holdsTraffic, "BackendsDegraded": holdsTraffic,
	"PortMismatch": holdsTraffic, "LoadBalancerPending": holdsTraffic,
	"BackendMissing": holdsSender, "Scaling": holdsScaling,
}

// canHave reports whether an object like id can have the symptom of
// family.
func canHave(id inventory.EntityID, family string) bool {
	kind := inventory.Kind(strings.ToLower(string(id.Kind)))
	custom := id.Group != ""
	if !custom && !apiKind(kind) && kind != kube.KindContainer {
		return false
	}
	switch familyHolders[family] {
	case holdsCode:
		return custom || incident.IsApplicationKind(kind)
	case holdsNode:
		return kind == kube.KindNode
	case holdsTraffic:
		return kind == kube.KindService
	case holdsSender:
		return !incident.IsApplicationKind(kind) && kind != kube.KindNode &&
			kind != kube.KindContainer
	case holdsScaling:
		return custom || kind == kube.KindHPA
	case holdsAny:
		return kind != kube.KindContainer || custom
	}
	return false
}

// apiKind reports a built-in kind the API serves: not a virtual root
// such as a finalizer, a scheduling reason or an image.
func apiKind(kind inventory.Kind) bool {
	if notKubectl[kind] || kind == explain.KindFinalizer ||
		kind == explain.KindFailureSignature {
		return false
	}
	_, ok := kindSpellings()[string(kind)]
	return ok
}

// holderOf names what has the symptom of family, as "What's wrong?"
// opens: the root when it can have it, else the failing members that
// can, those of the incident's own mode first. One member is named;
// several are "Each of 3 objects in shop", their names after the
// symptom (holderNames). "" when nothing can have it.
func holderOf(p incident.Incident, family string) (string, []string) {
	if p.Root.Name != "" && canHave(p.Root, family) {
		return subjectOf(p), nil
	}
	held := symptomMembers(p, family)
	switch len(held) {
	case 0:
		return "", nil
	case 1:
		return rootWords(held[0]), nil
	}
	return "Each of " + objectsIn(held), namesOf(held)
}

// symptomMembers are the distinct failing members other than the root
// that can have the symptom of family: those of the incident's own mode
// when there are any, else those of its family.
func symptomMembers(p incident.Incident, family string) []inventory.EntityID {
	var exact, same []inventory.EntityID
	seen := map[inventory.EntityID]bool{}
	for _, f := range p.Members {
		id := f.Entity
		if f.Advisory || id == p.Root || seen[id] || !canHave(id, family) {
			continue
		}
		switch {
		case f.Mode == p.Mode && p.Mode != "":
			exact = append(exact, id)
		case string(f.Mode.Family()) == family || family == probeFamily:
			same = append(same, id)
		default:
			continue
		}
		seen[id] = true
	}
	if len(exact) == 0 {
		exact = same
	}
	sort.Slice(exact, func(i, j int) bool {
		return exact[i].String() < exact[j].String()
	})
	return exact
}

// objectsIn is "3 objects in shop", or "3 objects" across namespaces.
func objectsIn(ids []inventory.EntityID) string {
	out := fmt.Sprintf("%d objects", len(ids))
	if ns := oneNamespace(ids); ns != "" {
		out += " in " + ns
	}
	return out
}

// oneNamespace is the namespace all of ids share, "" when they differ
// or have none.
func oneNamespace(ids []inventory.EntityID) string {
	ns := ids[0].Namespace
	for _, id := range ids[1:] {
		if id.Namespace != ns {
			return ""
		}
	}
	return ns
}

// maxHolderNames bounds the objects "What's wrong?" names in a group.
const maxHolderNames = 3

// namesOf names objects without their namespace, the first few:
// "Role a", "Secret b", "and 2 more".
func namesOf(ids []inventory.EntityID) []string {
	var out []string
	for i, id := range ids {
		if i == maxHolderNames {
			return append(out, fmt.Sprintf("and %d more", len(ids)-i))
		}
		out = append(out, kindDisplay(id)+" "+id.Name)
	}
	return out
}

// holderNames is " (Role a, Secret b and 2 more)" after a symptom said
// of a group, nothing for one object.
func holderNames(names []string) string {
	switch n := len(names); {
	case n == 0:
		return ""
	case n == 1:
		return " (" + names[0] + ")"
	default:
		last := names[n-1]
		if !strings.HasPrefix(last, "and ") {
			last = "and " + last
		}
		return " (" + strings.Join(names[:n-1], ", ") + " " + last + ")"
	}
}
