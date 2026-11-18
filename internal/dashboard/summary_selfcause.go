package dashboard

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/redact"
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
		if !ownedName(p, root, f.Entity) {
			return false
		}
		owned++
	}
	return owned > 0 || root == p.Root
}

// ownedName reports the root itself, or an object of its own by the
// cluster's relations (incident.Belongs): its pods and containers, its
// autoscaler, a Service that selects only its pods. Without a relation
// (an older record) only what a controller creates and names after
// itself counts: a same-named object of another kind is not its own.
func ownedName(p incident.Incident, root, e inventory.EntityID) bool {
	if owned, known := p.Belongs.Of(root, e); known {
		return owned
	}
	return e.Namespace == root.Namespace && root.Name != "" &&
		createdKind(e.Kind) && (strings.HasPrefix(e.Name, root.Name+"-") ||
		strings.HasPrefix(e.Name, root.Name+"/"))
}

// createdKind reports a kind a controller creates and names after
// itself.
func createdKind(k inventory.Kind) bool {
	switch k {
	case kube.KindPod, kube.KindContainer, kube.KindJob,
		kube.KindReplicaSet:
		return true
	}
	return false
}

// selfCause says why a workload fails on its own: the setting the
// title blames, else the error its logs report, else how its
// container last stopped; "" with none of them, since "the failure is
// in it" says nothing the reader does not know.
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
	return ""
}

// runsCode reports a kind that runs code (incident.IsApplicationKind),
// the only kind a failure can be "in itself" of: a Secret, a Service
// or an APIService fails through what it holds or serves.
func runsCode(id inventory.EntityID) bool {
	return incident.IsApplicationKind(
		inventory.Kind(strings.ToLower(string(id.Kind))))
}

// othersFail reports a failing member that is not root itself: the
// root is then the cause of something else, and says so.
func othersFail(p incident.Incident, root inventory.EntityID) bool {
	for _, f := range p.Members {
		if !f.Advisory && f.Entity != root {
			return true
		}
	}
	return false
}

// ownFinding is what kwatch found on a root that runs no code and
// fails on its own, in its own words: "What kwatch found on the
// service db: Service has no ready backends." "" without a finding of
// its own.
func ownFinding(p incident.Incident, root inventory.EntityID) string {
	best := detection.Finding{}
	for _, f := range p.Members {
		if f.Advisory || f.Entity != root || bare(f.Summary) == "" {
			continue
		}
		if best.Summary == "" || f.Since.After(best.Since) {
			best = f
		}
	}
	if best.Summary == "" {
		return ""
	}
	return "What kwatch found on the " + strings.ToLower(kindDisplay(root)) +
		" " + root.Name + ": " + sentence(redact.Evidence(best.Summary))
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
