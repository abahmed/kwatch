package compose

import (
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
	lead := sentence{part: partLead, text: capitalName(subject,
		f.leadName(subject)+" "+resolution(f, subject)+".")}
	lasted := "it was " + downWord(p) + " for " +
		humanDuration(p.Resolved.Sub(p.Opened))
	detail := sentenceCase(lasted)
	if fix := fixPhrase(f); fix != "" {
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
