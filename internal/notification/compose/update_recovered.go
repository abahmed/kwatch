package compose

import (
	"slices"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// recoveredSentences say what recovered, once: a container that
// recovered with its pod is the pod recovering, and what is still open
// is said once however many members recovered.
func recoveredSentences(
	f caseFacts, ids []inventory.EntityID,
) []sentence {
	ids = withoutRecoveredParts(ids)
	switch len(ids) {
	case 0:
		return nil
	case 1:
		return []sentence{recoveredSentence(f, ids[0])}
	}
	home := leadSubject(f)
	if slices.Contains(ids, home) {
		// The subject recovering says it all; the rest are its parts.
		return []sentence{recoveredSentence(f, home)}
	}
	text := nameList(home, ids) + " have recovered"
	if open := len(failing(f.members)); open > 0 {
		text += "; " + f.leadName(home) + " still has " +
			plural(open, "open failure")
	}
	return []sentence{{part: partLead,
		text: capitalName(ids[0], text+".")}}
}

// withoutRecoveredParts drops the containers whose pod is among ids.
func withoutRecoveredParts(
	ids []inventory.EntityID,
) []inventory.EntityID {
	var out []inventory.EntityID
	for _, id := range ids {
		if id.Kind == kube.KindContainer {
			pod, _ := splitContainer(id.Name)
			if slices.Contains(ids, inventory.CoreID(kube.KindPod,
				id.Namespace, pod)) {
				continue
			}
		}
		out = append(out, id)
	}
	return out
}

// recoveredSentence says a member recovered and what is still open,
// naming the subject: "Service cart has recovered; payments in shop
// still has one open failure."
func recoveredSentence(f caseFacts, id inventory.EntityID) sentence {
	home := leadSubject(f)
	open := len(failing(f.members))
	if id == home || open == 0 {
		text := f.leadName(id) + " has recovered"
		if open > 0 {
			text += "; " + plural(open, "other failure") + " " +
				verb(open, "is", "are") + " still open"
		}
		return sentence{part: partLead, text: capitalName(id, text+".")}
	}
	text := nameFrom(home, id) + " has recovered; " + f.leadName(home) +
		" still has " + plural(open, "open failure")
	return sentence{part: partLead, text: capitalName(id, text+".")}
}
