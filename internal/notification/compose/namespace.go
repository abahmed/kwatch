package compose

import (
	"strconv"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/notification"
)

// OutageShared is what the failing workloads of a namespace outage have
// in common, as far as kwatch can observe it. Both fields may be empty.
// It is a fact to show, never a cause: nothing here says it broke them.
type OutageShared struct {
	// Node is the node every failing workload ran on.
	Node string
	// Change is a recent change in the namespace all of them were
	// written with.
	Change *inventory.Change
}

// NamespaceOutage describes many workloads of one namespace that failed
// at the same time without one cause tying them together.
type NamespaceOutage struct {
	Namespace string
	// Workloads is how many workloads the namespace has; zero when
	// unknown, which leaves the "of N" out.
	Workloads int
	Shared    OutageShared
}

// NamespaceOutageKey is the conversation key of the outage message of a
// namespace at at. It starts with the roll-up prefix so delivery threads
// the members' messages under it, and ends with the namespace so it
// never collides with a roll-up sent in the same moment.
func NamespaceOutageKey(namespace string, at time.Time) string {
	return RollupKey(at) + "/" + namespace
}

// NamespaceOutage is one message for the incidents of a namespace outage
// instead of one message each: how many workloads fail and since when,
// the worst first, and what they share. Like a roll-up it only names
// the incidents: each keeps its own conversation under this message,
// and RollupResolved closes it once all have resolved.
func (w Writer) NamespaceOutage(
	o NamespaceOutage, decisions []incident.Decision, now time.Time,
) notification.Message {
	decisions = append([]incident.Decision(nil), decisions...)
	sortByImpact(decisions)
	mark, status := notification.MarkerLow, notification.StatusLow
	if len(decisions) > 0 {
		mark = marker(decisions[0].Incident)
		status = tierStatus(decisions[0].Incident.Tier)
	}
	sentences := []sentence{{part: partLead, text: outageLead(o,
		w.clusterTag(), decisions)}}
	sentences = append(sentences, digestTitles(decisions, now, "")...)
	sentences = append(sentences, sharedSentences(o.Shared)...)
	sentences = append(sentences, sentence{part: partAction,
		text: eachOwnMessage})
	msg := notification.Message{
		Key:      NamespaceOutageKey(o.Namespace, now),
		Revision: 1, Status: status, Opens: true,
		Route: summaryRoute(decisions, nil),
	}
	fill(&msg, mark, sentences)
	return msg
}

// outageLead is "shop: 12 of 15 workloads failing since 10:02".
func outageLead(
	o NamespaceOutage, cluster string, ds []incident.Decision,
) string {
	count := strconv.Itoa(len(ds))
	if o.Workloads >= len(ds) {
		count += " of " + strconv.Itoa(o.Workloads)
	}
	lead := o.Namespace + cluster + ": " + count + " " +
		verb(len(ds), "workload", "workloads") + " failing"
	if since := outageSince(ds); !since.IsZero() {
		lead += " since " + clock(since)
	}
	return lead
}

// outageSince is when the first of the incidents opened.
func outageSince(ds []incident.Decision) time.Time {
	var first time.Time
	for _, d := range ds {
		if opened := d.Incident.Opened; first.IsZero() ||
			opened.Before(first) {
			first = opened
		}
	}
	return first
}

// sharedSentences states what the workloads have in common, without
// saying it is the cause.
func sharedSentences(s OutageShared) []sentence {
	var out []sentence
	if s.Node != "" {
		out = append(out, sentence{part: partProof,
			text: "They all ran on node " + s.Node + "."})
	}
	if s.Change != nil {
		out = append(out, sentence{part: partProof,
			text: "Shortly before, " + changeFact(*s.Change) + "."})
	}
	return out
}
