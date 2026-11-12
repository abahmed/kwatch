package dashboard

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// configFamily is the mode family of a workload broken by its own
// settings ("Config.ProbePortMismatch").
const configFamily detection.Mode = "Config"

// noOutsideCause opens the cause sentence of a workload that fails on
// its own.
const noOutsideCause = "kwatch hasn't found an outside cause"

// failsOnItsOwn reports a cause that is the incident's own workload:
// every failing object is the root itself or one of its pods, so
// naming the root as their "common cause" says nothing.
func failsOnItsOwn(p incident.Incident, root inventory.EntityID) bool {
	if root.Name == "" || root.Kind == rootcause.KindScheduling {
		return false
	}
	owned := 0
	for _, f := range p.Members {
		if f.Advisory {
			continue
		}
		if !ownedName(root, f.Entity) {
			return false
		}
		owned++
	}
	return owned > 0 || root == p.Root
}

// ownedName reports the root itself, or an object of its own named
// after it: its pods and containers, or a Service of the same name.
func ownedName(root, e inventory.EntityID) bool {
	if e == root {
		return true
	}
	return e.Namespace == root.Namespace && root.Name != "" &&
		(e.Name == root.Name || strings.HasPrefix(e.Name, root.Name+"-") ||
			strings.HasPrefix(e.Name, root.Name+"/"))
}

// selfCause says why a workload fails on its own: the setting the
// title blames, else the error its logs report, else how its
// container last stopped, else that nothing outside it was found.
func selfCause(d *IncidentDetail, p incident.Incident) string {
	if own := configCause(d, p); own != "" {
		return own
	}
	if logs := logProof(d); len(logs) > 0 {
		line := strings.TrimRight(logs[0].Detail, ". ")
		line, _ = shortText(line, maxProofDetail)
		return noOutsideCause + "; its own logs say: “" + line + "”."
	}
	if exits := exitRows(d); len(exits) > 0 {
		proof := exitProof(d)[0]
		return noOutsideCause + "; its container " + exits[0].Container +
			" last stopped: " + proof.Detail + "."
	}
	return noOutsideCause + ": the failure is in the " +
		strings.ToLower(kindDisplay(p.Root)) + " " + p.Root.Name +
		" itself."
}

// configCause is the setting a self-configuration cause blames, as the
// title states it after "because": "Its probe checks a port the
// container does not listen on." "" for any other cause.
func configCause(d *IncidentDetail, p incident.Incident) string {
	if p.Cause.Mode.Family() != configFamily {
		return ""
	}
	title := d.FullTitle
	if title == "" {
		title = d.Title
	}
	_, why, found := strings.Cut(title, " because ")
	if !found || strings.HasSuffix(why, "…") {
		return ""
	}
	return sentence(upperFirst(why))
}

// upperFirst is s with a capital first letter.
func upperFirst(s string) string {
	r, n := utf8.DecodeRuneInString(s)
	if n == 0 {
		return s
	}
	return string(unicode.ToUpper(r)) + s[n:]
}

// schedulingCause words a virtual scheduling root: "The scheduler
// can't place the pod: no node matches its node affinity."
func schedulingCause(p incident.Incident, root inventory.EntityID) string {
	what := "the pod"
	if len(p.Members) > 1 {
		what = "the pods"
	}
	return "The scheduler can't place " + what + ": " +
		rootcause.SchedulingWords(root.Name) + "."
}
