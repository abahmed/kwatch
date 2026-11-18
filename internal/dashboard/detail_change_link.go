package dashboard

import (
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
)

// endedWhy is the sentence for a change that records the end of a link
// to a failing object, its pods' workload or what they use: the link
// no longer shows in the model, so the change says what it was. ok is
// false for any other change, which the other sentences explain.
func (l *changeLinks) endedWhy(change inventory.Change) (string, bool) {
	words, target, ok := incident.EndedLink(change)
	if !ok {
		return "", false
	}
	var what string
	switch {
	case l.failing[target] || l.pods[target]:
		what = "a failing object"
	case l.owners[target]:
		what = "the workload the failing pods belong to"
	case l.mounted[target] || l.used[target]:
		what = "a " + kindWord(target.Kind) + " the failing pods use"
	default:
		return "", false
	}
	return "This " + kindWord(change.Entity.Kind) + " " + words + " " +
		what + ".", true
}
