package compose

import (
	"sort"
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/redact"
	"github.com/abahmed/kwatch/internal/rootcause"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// handlerClause says who should remove the finalizers and why that does
// not happen. A controller is named only when the cause names it: the
// cluster says which workload serves the finalizer's group, never a
// look-alike.
func handlerClause(
	cause *rootcause.CauseRecord, held []detection.Finding, names []string,
) string {
	it := "it"
	if len(names) > 1 {
		it = "them"
	}
	switch {
	case cause.Rule == "finalizer-handler-stopped" &&
		cause.Mode.Within(explain.ModeHandlerScaledDown):
		return "the controller that handles " + it + ", " +
			workloadRef(cause.Root) + ", is scaled to 0"
	case cause.Rule == "finalizer-handler-stopped":
		return "the controller that handles " + it + ", " +
			workloadRef(cause.Root) + ", has no ready replica"
	case cause.Mode.Within(explain.ModeFinalizerHandlerRuns):
		return "a controller that appears to handle " + it +
			" is running, so kwatch cannot say why it does not remove " + it
	}
	group := heldGroup(held, names)
	text := "no controller in the cluster is known to handle " + it
	if group != "" {
		text += " (API group " + group + ")"
	}
	return text + lookAlikeClause(cause, group)
}

// lookAlikeClause names the workloads that are named like the controller
// but that nothing ties to the group: what kwatch saw and could not
// confirm. Empty when there are none.
func lookAlikeClause(cause *rootcause.CauseRecord, group string) string {
	var names []string
	for _, p := range cause.Proof {
		if p.Code == rootcause.ProofNamedLikeHandler {
			names = p.Fields
		}
	}
	if len(names) == 0 {
		return ""
	}
	serves := "serves it"
	if group != "" {
		serves = "serves group " + group
	}
	if len(names) == 1 {
		return "; " + names[0] + " is named like it, but nothing " +
			"confirms it " + serves
	}
	return "; " + joinAnd(names, maxNamedFinalizers) + " are named like " +
		"it, but nothing confirms they " + strings.Replace(serves,
		"serves", "serve", 1)
}

// heldGroup is the API group of the finalizers: the custom group the
// held objects themselves belong to, else the domain of the first
// finalizer. Empty when neither looks like a group (a finalizer such as
// "external-attacher/ebs-csi-example-com" names none).
func heldGroup(held []detection.Finding, names []string) string {
	for _, m := range held {
		if m.Entity.Group != "" {
			return m.Entity.Group
		}
	}
	for _, name := range names {
		if domain, _, ok := strings.Cut(name, "/"); ok &&
			strings.Contains(domain, ".") {
			return domain
		}
	}
	return ""
}

// reasonSource is where a held object says why it cannot be removed.
type reasonSource struct {
	// reason is the reason of the finding that carries the evidence.
	reason string
	// label is the evidence label.
	label string
	// words say what the quote is, after its subject.
	words func(subject string) string
}

// reasonSources are tried in order: what the object's own status says
// beats an event about it.
var reasonSources = []reasonSource{
	{reasons.VolumeDetachFailure, "detach error",
		func(subject string) string {
			return "the status of " + subject +
				" reports a detach error: "
		}},
	{reasons.StuckDeleting, detection.EvidenceLastWarning,
		func(subject string) string {
			return "the latest Warning event on " + subject + " says: "
		}},
}

// heldReason quotes why a held object cannot go, in the words of the
// object or of its controller: a status error first, else the latest
// Warning event. It says which object when several are held. Empty when
// none says anything.
func heldReason(held, members []detection.Finding) string {
	sorted := append([]detection.Finding(nil), held...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Entity.String() < sorted[j].Entity.String()
	})
	for _, source := range reasonSources {
		for _, h := range sorted {
			message := reasonOf(h, members, source)
			if message == "" {
				continue
			}
			subject := "it"
			if len(held) > 1 {
				subject = shortName(h.Entity)
			}
			return source.words(subject) + quotedTail(message)
		}
	}
	return ""
}

// reasonOf is the message a source gives for a held object.
func reasonOf(
	held detection.Finding, members []detection.Finding, source reasonSource,
) string {
	for _, m := range members {
		if m.Entity == held.Entity && m.Reason == source.reason {
			if message := evidence(m, source.label); message != "" {
				return message
			}
		}
	}
	return ""
}

// quotedTail quotes a message from its end when it is too long: a
// wrapped error states its cause last, so the end is the part that says
// why. What it quotes is the controller's own text, with credentials
// removed.
func quotedTail(text string) string {
	runes := []rune(strings.TrimSpace(
		dropQuoteRunes(redact.Credentials(text))))
	if len(runes) <= maxQuote {
		return markQuote(string(runes))
	}
	tail := string(runes[len(runes)-maxQuote:])
	if i := strings.Index(tail, " "); i >= 0 && i < maxQuote/2 {
		tail = tail[i+1:]
	}
	return markQuote("…" + strings.TrimLeft(tail, " ,;:."))
}

// stuckObjectLead words a deletion that one object has waited for with no
// stated cause, when the object's controller wrote why it cannot finish:
// "Service kube-addons/ingress is stuck deleting: finalizers
// service.k8s.aws/resources hold it; the latest Warning event on it
// says: …". Without such a message the usual wording stands.
func stuckObjectLead(f caseFacts) (string, bool) {
	if f.p.Cause != nil || !f.ok ||
		f.lead.Mode != detection.ModeStuckDeleting {
		return "", false
	}
	held := heldObjects(f.members)
	if len(held) != 1 || held[0].Entity != f.p.Root {
		return "", false
	}
	reason := heldReason(held, f.members)
	if reason == "" {
		return "", false
	}
	return ownLead(f) + "; " + reason, true
}
