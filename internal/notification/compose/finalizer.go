package compose

import (
	"sort"
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// maxNamedFinalizers bounds how many finalizers the lead names.
const maxNamedFinalizers = 3

// finalizerCause reports a cause that is a finalizer nothing removes, or
// the controller that should remove it.
func finalizerCause(cause *rootcause.CauseRecord) bool {
	return cause != nil && (cause.Rule == "finalizer-unhandled" ||
		cause.Rule == "finalizer-handler-stopped")
}

// finalizerLead says that objects have been stuck deleting for good: the
// finalizer that holds them is never removed, and why. "Six objects in
// shop have been stuck deleting since Dec 4: their finalizer
// example.com/cleanup is never removed; the controller that handles it,
// deployment shop/widget-ctl, is scaled to 0." A held object that says
// why itself is quoted after it (see heldReason).
func finalizerLead(f caseFacts) (string, bool) {
	if !finalizerCause(f.p.Cause) {
		return stuckObjectLead(f)
	}
	held := heldObjects(f.members)
	if len(held) == 0 {
		return "", false
	}
	text := upperFirst(plural(len(held), "object")) +
		heldNames(held) + " " + heldPlace(f, held) +
		" " + verb(len(held), "has", "have") + " been stuck deleting" +
		heldSince(f, held) + ": "
	names := heldFinalizers(held)
	their := "their"
	if len(held) == 1 {
		their = "its"
	}
	if len(names) == 1 {
		text += their + " finalizer " + names[0] + " is never removed"
	} else {
		text += their + " finalizers " + joinAnd(names,
			maxNamedFinalizers) + " are never removed"
	}
	text += "; " + handlerClause(f.p.Cause, held, names)
	if reason := heldReason(held, f.members); reason != "" {
		text += "; " + reason
	}
	return text + ".", true
}

// heldObjects are the members whose deletion a finalizer holds.
func heldObjects(members []detection.Finding) []detection.Finding {
	var out []detection.Finding
	seen := map[inventory.EntityID]bool{}
	for _, m := range members {
		if m.Mode == detection.ModeStuckDeleting && !seen[m.Entity] {
			seen[m.Entity] = true
			out = append(out, m)
		}
	}
	return out
}

// maxNamedHeld is how many held objects the lead names; more are only
// counted.
const maxNamedHeld = 3

// heldNames is " (service account a, role b and role binding c)" for a
// few objects, so the reader can tell what is stuck; nothing for more.
func heldNames(held []detection.Finding) string {
	if len(held) > maxNamedHeld {
		return ""
	}
	var names []string
	for _, m := range held {
		names = append(names, shortName(m.Entity))
	}
	sort.Strings(names)
	return " (" + joinAnd(names, maxNamedHeld) + ")"
}

// heldPlace is "in shop" for objects of one namespace and "in 3
// namespaces" for more; objects with no namespace are not placed.
func heldPlace(f caseFacts, held []detection.Finding) string {
	spaces := map[string]bool{}
	for _, m := range held {
		if m.Entity.Namespace != "" {
			spaces[m.Entity.Namespace] = true
		}
	}
	switch len(spaces) {
	case 0:
		return "of the cluster" + f.clusterTag()
	case 1:
		for name := range spaces {
			return "in " + name + f.clusterTag()
		}
	}
	return "in " + plural(len(spaces), "namespace") + f.clusterTag()
}

// heldSince is " since Dec 4, 2025", from the oldest deletion, or from
// when the problem started if that was earlier (see StartedAt). The
// year is said unless it is this year.
func heldSince(f caseFacts, held []detection.Finding) string {
	first := StartedAt(f.p)
	for _, m := range held {
		if !m.Since.IsZero() && (first.IsZero() || m.Since.Before(first)) {
			first = m.Since
		}
	}
	if first.IsZero() {
		return ""
	}
	return " since " + shortDate(first, f.now)
}

// heldFinalizers lists the distinct finalizers named by the objects,
// without the ones Kubernetes runs itself.
func heldFinalizers(held []detection.Finding) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range held {
		for _, name := range strings.Split(evidence(m, detection.EvidenceFinalizers),
			", ") {
			if name != "" && !seen[name] {
				seen[name] = true
				out = append(out, name)
			}
		}
	}
	sort.Strings(out)
	return out
}

// workloadRef is "deployment shop/widget-ctl".
func workloadRef(id inventory.EntityID) string {
	return kindWord(id.Kind) + " " + id.Namespace + "/" + id.Name
}
