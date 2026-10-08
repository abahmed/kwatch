package compose

import (
	"sort"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/rootcause"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
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
// deployment shop/widget-ctl, is scaled to 0."
func finalizerLead(f caseFacts) (string, bool) {
	if !finalizerCause(f.p.Cause) {
		return "", false
	}
	held := heldObjects(f.members)
	if len(held) == 0 {
		return "", false
	}
	text := upperFirst(plural(len(held), "object")) + " " +
		heldPlace(f, held) +
		" " + verb(len(held), "has", "have") + " been stuck deleting" +
		heldSince(held) + ": "
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
	return text + "; " + handlerClause(f.p.Cause, len(names) > 1), true
}

// heldObjects are the members whose deletion a finalizer holds.
func heldObjects(members []detection.Finding) []detection.Finding {
	var out []detection.Finding
	for _, m := range members {
		if m.Mode == detection.ModeStuckDeleting {
			out = append(out, m)
		}
	}
	return out
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

// heldSince is " since Dec 4", from the oldest deletion.
func heldSince(held []detection.Finding) string {
	var first time.Time
	for _, m := range held {
		if !m.Since.IsZero() && (first.IsZero() || m.Since.Before(first)) {
			first = m.Since
		}
	}
	if first.IsZero() {
		return ""
	}
	return " since " + first.UTC().Format("Jan 2")
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

// handlerClause says who should remove the finalizers and why that does
// not happen; several says there is more than one finalizer.
func handlerClause(cause *rootcause.CauseRecord, several bool) string {
	it := "it"
	if several {
		it = "them"
	}
	switch {
	case cause.Rule == "finalizer-handler-stopped" &&
		cause.Mode.Within(explain.ModeHandlerScaledDown):
		return "the controller that handles " + it + ", " +
			workloadRef(cause.Root) + ", is scaled to 0."
	case cause.Rule == "finalizer-handler-stopped":
		return "the controller that handles " + it + ", " +
			workloadRef(cause.Root) + ", has no ready replica."
	case cause.Mode.Within(explain.ModeFinalizerHandlerRuns):
		return "a controller that looks like its handler is running, " +
			"so kwatch cannot say why it does not remove " + it + "."
	}
	return "no running controller found for " + it + "."
}

// workloadRef is "deployment shop/widget-ctl".
func workloadRef(id inventory.EntityID) string {
	return kindWord(id.Kind) + " " + id.Namespace + "/" + id.Name
}
