package compose

import (
	"strings"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
)

// recentChangeSentence says what changed lately next to a failure that
// has no cause: the facts a reader would otherwise go and look up. It
// states them without blaming anyone: "In the last 30 minutes in shop:
// the api rollout started at 21:12; bob changed config map app-config
// at 21:10."
func recentChangeSentence(f caseFacts) []sentence {
	if len(f.changes) == 0 || f.p.Root.Namespace == "" {
		return nil
	}
	items := make([]string, 0, len(f.changes))
	for _, change := range f.changes {
		items = append(items, changeFact(change))
	}
	return []sentence{{part: partCause, weight: -1, text: "In the last 30 " +
		"minutes in " +
		f.p.Root.Namespace + ": " + strings.Join(items, "; ") + "."}}
}

// changeFact is one change as a clause: who did what to which object, at
// what time. The actor is left out when the change has none.
func changeFact(change inventory.Change) string {
	who := person(change.Actor)
	if who != "" {
		who += " "
	}
	at := " at " + clock(change.At)
	name := shortName(change.Entity)
	if edits := valueEdits(change); len(edits) > 0 &&
		!change.Created && !change.Deleted {
		return name + ": " + editList(edits) + at + authoredBy(who)
	}
	switch {
	case change.Deleted:
		return passive(who, "deleted", name, "was deleted") + at
	case change.Created:
		return passive(who, "created", name, "was created") + at
	}
	switch change.Classify() {
	case inventory.ClassRollout, inventory.ClassImage:
		if incident.IsWorkload(change.Entity.Kind) {
			return who + rolloutWords(who, change.Entity) + at
		}
	}
	return passive(who, "changed", name, "changed") + at
}

// passive writes "bob changed x" with an actor and "x changed" without.
func passive(who, verb, name, bare string) string {
	if who == "" {
		return name + " " + bare
	}
	return who + verb + " " + name
}

// rolloutWords is "started the api rollout" for a person, and "the api
// rollout started" when nobody is named.
func rolloutWords(who string, workload inventory.EntityID) string {
	if who == "" {
		return "the " + workload.Name + " rollout started"
	}
	return "started the " + workload.Name + " rollout"
}

// person is the actor when it is a person or a tool of theirs, and empty
// for Kubernetes' own controllers, which are nobody's change.
func person(actor string) string {
	if strings.HasPrefix(actor, "kube-") ||
		strings.HasPrefix(actor, "kubelet") {
		return ""
	}
	return actor
}

// authoredBy is " by alice" for a named actor and nothing for nobody.
func authoredBy(who string) string {
	if who == "" {
		return ""
	}
	return " by " + strings.TrimSpace(who)
}
