package compose

import (
	"slices"
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
		return marker(p), movedNote(f)
	}
	subject := leadSubject(f)
	if rest, ok := strings.CutPrefix(string(f.reason),
		incident.StoppedTrackingPrefix); ok {
		return notification.MarkerResolved, stoppedTracking(f, subject, rest)
	}
	lead := sentence{part: partLead, text: capitalName(subject,
		f.leadName(subject)+" "+resolution(f, subject)+".")}
	lasted := "it was " + downWord(p) + " for " +
		humanDuration(failedSpan(p)) + resolvedCause(f, subject)
	detail := sentenceCase(lasted)
	if fix := resolveFixer(f, subject); fix != "" {
		detail = fix + "; " + lasted + "."
	}
	out := []sentence{lead, {part: partProof, text: detail}}
	if affected := affectedSentence(p); affected != "" {
		out = append(out, sentence{part: partConsequence, text: affected})
	}
	if !removed(f, subject) {
		out = append(out, changedBeforeRecoverySentences(f)...)
	}
	if last := lastAffected(f, subject); last != "" {
		out = append(out, sentence{part: partProof, text: last})
	}
	return notification.MarkerResolved, out
}

// lastAffected names the root an incident ended on when its resolve
// speaks of the subject a listing announced and the root moved since:
// "Last affected: deployment draftor in staging."
func lastAffected(f caseFacts, subject inventory.EntityID) string {
	if !f.asAnnounced || zeroID(f.p.Root) || f.p.Root == subject {
		return ""
	}
	return lastAffectedLead + describe(f.p.Root) + "."
}

// lastAffectedLead starts the line naming the root a listed incident
// ended on; Recovered keeps that line beside a carried lead.
const lastAffectedLead = "Last affected: "

// affectedSentence says how many pods failed at the widest: "Three
// pods were affected." One pod is the subject itself, so it says
// nothing.
func affectedSentence(p incident.Incident) string {
	if n := p.Journal.Widest.Pods; n > 1 {
		return upperFirst(plural(n, "pod")) + " were affected."
	}
	return ""
}

// resolveFixer names who fixed it; nobody fixed a deletion.
func resolveFixer(f caseFacts, subject inventory.EntityID) string {
	if removed(f, subject) {
		return ""
	}
	if fix := fixedByPhrase(f); fix != "" {
		return fix
	}
	return fixPhrase(f)
}

// removed reports that the subject ended because it was deleted, alone
// or with its namespace, not because it recovered. A node has its own
// wording.
func removed(f caseFacts, subject inventory.EntityID) bool {
	if subject.Kind == kube.KindNode {
		return false
	}
	return slices.Contains(f.p.Removed, subject) ||
		(f.p.NamespaceRemoved && subject.Namespace == f.p.Root.Namespace)
}

// resolution says how the subject ended: gone, replaced or healthy.
func resolution(f caseFacts, subject inventory.EntityID) string {
	switch {
	case removed(f, subject) && f.p.NamespaceRemoved:
		return "was deleted with its namespace"
	case removed(f, subject):
		return "was deleted"
	}
	if subject.Kind != kube.KindNode {
		return "is healthy again"
	}
	switch {
	case nodeDeleted(f, subject):
		return "is gone and its pods were rescheduled"
	case f.p.Fix == incident.FixNodeReplaced:
		return "was replaced and its pods were rescheduled"
	}
	return nodeRecovered(f)
}

// nodeDeleted reports that the change which ended the incident deleted
// the node.
func nodeDeleted(f caseFacts, node inventory.EntityID) bool {
	return f.fix != nil && f.fix.Deleted && f.fix.Entity == node
}

// downWord is "down" for an incident that paged, "failing" otherwise. An
// incident moved to another root was paged for the first one: what the
// current root did is "failing", which it does not overstate.
func downWord(p incident.Incident) string {
	if p.Tier == incident.Page && !movedAway(p) {
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

// stoppedTracking closes an incident that ended at its still-broken cap,
// so the message must not say it is healthy. Its lead names the
// condition that held: the workload short of replicas, or, with
// incident.StillCrashingNote, a container that keeps crashing; a
// subject that has no replicas "has not recovered". rest is
// the reason after its prefix, "2h; coverage check continues". The
// warning that it is still not healthy is a follow-up, not proof, so
// a Community message keeps it.
func stoppedTracking(
	f caseFacts, subject inventory.EntityID, rest string,
) []sentence {
	span, _, _ := strings.Cut(rest, ";")
	if strings.HasSuffix(rest, incident.UnconfirmedNote) {
		return unconfirmedRecovery(f, subject, span)
	}
	state := " has not recovered."
	switch {
	case strings.HasSuffix(rest, incident.StillCrashingNote):
		state = " still has a container that keeps crashing."
	case incident.IsWorkload(subject.Kind):
		state = " is still short of replicas."
	}
	return []sentence{
		{part: partLead, text: capitalName(subject,
			f.leadName(subject)+state)},
		{part: partFollowUp, text: "kwatch stopped tracking this " +
			"after " + strings.TrimSpace(span) + "; it is still not " +
			"healthy, so check it yourself."},
	}
}

// unconfirmedRecovery words the end of a hold for missing data: kwatch
// waited, could not see the object, and says so instead of claiming a
// recovery or a failure.
func unconfirmedRecovery(
	f caseFacts, subject inventory.EntityID, span string,
) []sentence {
	return []sentence{
		{part: partLead, text: capitalName(subject,
			f.leadName(subject)+": kwatch could not confirm it "+
				"recovered.")},
		{part: partFollowUp, text: "kwatch could not observe it for " +
			strings.TrimSpace(span) + " and stopped checking; look " +
			"at it yourself."},
	}
}
