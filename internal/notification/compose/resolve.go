package compose

import (
	"strings"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/notification"
)

// resolveNote closes an incident. Its lead follows how the incident
// ended: "payments in shop is healthy again." after a fix or a
// recovery, "Node n1 is gone and its pods were rescheduled." after a
// node was deleted. A superseded incident never claims that anything
// is healthy: its failures now belong to another incident.
func resolveNote(f caseFacts) (string, []sentence) {
	p := f.p
	if p.SupersededBy != "" {
		return marker(p), []sentence{{part: partLead,
			text: "Cause revised: the failures of " + f.leadName(p.Root) +
				" are now tracked in incident " + p.SupersededBy +
				" and are not resolved."}}
	}
	subject := leadSubject(f)
	if rest, ok := strings.CutPrefix(string(f.reason),
		incident.StoppedTrackingPrefix); ok {
		return notification.MarkerResolved, stoppedTracking(f, subject, rest)
	}
	lead := sentence{part: partLead, text: capitalName(subject,
		f.leadName(subject)+" "+resolution(f, subject)+".")}
	lasted := "it was " + downWord(p) + " for " +
		humanDuration(p.Resolved.Sub(p.Opened)) + resolvedCause(f, subject)
	detail := sentenceCase(lasted)
	if fix := fixedByPhrase(f); fix != "" {
		detail = fix + "; " + lasted + "."
	} else if fix := fixPhrase(f); fix != "" {
		detail = fix + "; " + lasted + "."
	}
	return notification.MarkerResolved, []sentence{lead,
		{part: partProof, text: detail}}
}

// resolution says how the subject ended: gone, replaced or healthy.
func resolution(f caseFacts, subject inventory.EntityID) string {
	if subject.Kind != kube.KindNode {
		return "is healthy again"
	}
	switch {
	case nodeDeleted(f, subject):
		return "is gone and its pods were rescheduled"
	case f.p.Fix == incident.FixNodeReplaced:
		return "was replaced and its pods were rescheduled"
	}
	return "is ready again"
}

// nodeDeleted reports that the change which ended the incident deleted
// the node.
func nodeDeleted(f caseFacts, node inventory.EntityID) bool {
	return f.fix != nil && f.fix.Deleted && f.fix.Entity == node
}

func downWord(p incident.Incident) string {
	if p.Tier == incident.Page {
		return "down"
	}
	return "failing"
}

// fixPhrase says who fixed it and how, when the fixing change is known:
// "alice rolled back to revision 13 at 14:09". A deleted node and the
// node controller's own taints were already said or are nobody's fix.
func fixPhrase(f caseFacts) string {
	fix, p := f.fix, f.p
	if fix == nil || systemChange(*fix) || nodeDeleted(f, fix.Entity) {
		return ""
	}
	who := actorOr(fix.Actor, "Someone")
	at := " at " + clock(fix.At)
	switch {
	case fix.Revision != "" && p.Cause != nil &&
		p.Cause.RollbackRevision == fix.Revision:
		return who + " rolled back to revision " + fix.Revision + at
	case fix.Revision != "":
		return who + " rolled out revision " + fix.Revision + at
	case len(fix.Fields) > 0:
		return who + " changed " + fieldWords(fix.Fields[0].Path) + at
	}
	return who + " changed " + nameFrom(p.Root, fix.Entity) + at
}

// resolvedCause says, in the resolve, what the incident was blamed on:
// " because node n3 was low on memory". The resolve is the message
// people read when they come back, so it carries the cause. A workload
// blamed for itself names none.
func resolvedCause(f caseFacts, subject inventory.EntityID) string {
	cause := f.p.Cause
	if cause == nil || cause.Rule == "self" || cause.Root == subject {
		return ""
	}
	return " because " + pastTense(causePhrase(cause, subject))
}

// pastTense moves the cause's verb into the past: "is low on memory"
// becomes "was low on memory".
func pastTense(text string) string {
	for _, r := range [][2]string{
		{" is ", " was "}, {" are ", " were "}, {" has ", " had "},
		{" does ", " did "}, {" keeps ", " kept "}, {" refuses ", " refused "},
		{" cannot ", " could not "},
	} {
		done := false
		text = outsideQuotes(text, func(part string) string {
			if done || !strings.Contains(part, r[0]) {
				return part
			}
			done = true
			return strings.Replace(part, r[0], r[1], 1)
		})
	}
	return text
}

// stoppedTracking closes an incident that ended at its still-broken cap:
// the workload is still short of replicas, so the message must not say it
// is healthy. rest is the reason after its prefix, "2h; coverage check
// continues".
func stoppedTracking(
	f caseFacts, subject inventory.EntityID, rest string,
) []sentence {
	span, _, _ := strings.Cut(rest, ";")
	return []sentence{
		{part: partLead, text: capitalName(subject, f.leadName(subject)+
			" is still short of replicas.")},
		{part: partProof, text: "kwatch stopped tracking this incident " +
			"after " + strings.TrimSpace(span) + "; the coverage check " +
			"keeps watching it."},
	}
}
