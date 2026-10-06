package incident

import (
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// Fix says how an earlier occurrence of an incident ended. Values are
// stable: they are persisted.
type Fix string

// Fixes, derived from the change outcomes seen while the incident was
// open.
const (
	FixRollback     Fix = "rollback"
	FixConfig       Fix = "config fix"
	FixNodeReplaced Fix = "node replaced"
	FixChange       Fix = "change"
	FixNone         Fix = "recovered without change"
)

// maxHistory bounds the earlier occurrences kept per incident.
const maxHistory = 10

// Occurrence is one earlier, resolved occurrence of the same root.
type Occurrence struct {
	Mode detection.Mode `json:",omitempty"`
	// Modes are all the modes the occurrence's members had, Mode
	// included; older records have none.
	Modes    []detection.Mode `json:",omitempty"`
	Opened   time.Time
	Resolved time.Time
	Fix      Fix
	// Heard is true when the occurrence was announced. A blip that
	// recovered inside its settle, or a root another incident took over
	// first, is remembered for routines and flapping but is not "the
	// second time this week" to anyone.
	Heard bool `json:",omitempty"`
	// Trigger is what the occurrence's cause was blamed on, as one of
	// the TriggerOf words, or empty when none was named.
	Trigger string `json:",omitempty"`
	// Page is true when the occurrence paged, or would have if a repeat
	// had not been held. A page that re-opens soon after one is a
	// repeat of the same outage.
	Page bool `json:",omitempty"`
	// Digested is when a digest last listed the occurrence.
	Digested time.Time `json:",omitempty"`
}

// Triggers are the kinds of cause a recurrence can share.
const (
	TriggerRollout = "rollout"
	TriggerConfig  = "config change"
	TriggerNode    = "node"
)

// TriggerOf classifies a cause as one of the triggers, or "".
func TriggerOf(cause *rootcause.CauseRecord) string {
	if cause == nil {
		return ""
	}
	switch cause.Rule {
	case "rollout", "own-change":
		return TriggerRollout
	case "config-missing-or-changed", "configmap-missing-or-changed":
		if cause.Change != nil {
			return TriggerConfig
		}
	}
	if cause.Root.Kind == kube.KindNode {
		return TriggerNode
	}
	return ""
}

// Duration is how long the occurrence lasted.
func (o Occurrence) Duration() time.Duration {
	return o.Resolved.Sub(o.Opened)
}

// Describe renders "same as Tue 14:02, fixed by rollback" or "same as
// Tue 14:02, recovered without change". Times are UTC.
func (o Occurrence) Describe() string {
	when := "same as " + o.Opened.UTC().Format("Mon 15:04")
	switch o.Fix {
	case "":
		return when
	case FixNone:
		return when + ", " + string(FixNone)
	}
	return when + ", fixed by " + string(o.Fix)
}

// LastOccurrence returns the most recent earlier occurrence with the
// incident's mode, for messages such as "same as Tue 14:02, fixed by
// rollback". An incident without a mode matches any occurrence.
func (inc *Incident) LastOccurrence() (Occurrence, bool) {
	for i := len(inc.History) - 1; i >= 0; i-- {
		o := inc.History[i]
		if inc.Mode == "" || o.Mode == "" || o.Mode == inc.Mode {
			return o, true
		}
	}
	return Occurrence{}, false
}

// occurrenceOf closes p as one occurrence.
func occurrenceOf(p *Incident) Occurrence {
	return Occurrence{
		Mode: p.Mode, Modes: p.modeList(), Opened: p.Opened,
		Resolved: p.Resolved, Fix: p.Fix,
		Heard: p.sent.told, Trigger: TriggerOf(p.Cause),
		Page:     pagedOccurrence(p),
		Digested: p.DigestedAt,
	}
}

// pagedOccurrence reports a page that people (or the pagers) got: an
// announcement that was decided but dropped or held and never sent paged
// nobody, so it must not suppress the next real page as a "repeat".
func pagedOccurrence(p *Incident) bool {
	return !p.Announced.IsZero() && isPage(p) &&
		(p.sent.told || p.Delivery.OpenAtPagers())
}

// appendHistory returns history with o appended, keeping the most recent
// maxHistory entries.
func appendHistory(history []Occurrence, o Occurrence) []Occurrence {
	out := append(append([]Occurrence(nil), history...), o)
	if over := len(out) - maxHistory; over > 0 {
		out = append(out[:0:0], out[over:]...)
	}
	return out
}

// incidentMode picks the mode of the root's own finding, or of the first
// member by key when the root has none. Configuration risks never name
// the mode: they come and go on their own, so a mode taken from one would
// differ between occurrences of the same failure and the problem would
// never be recognised as known. They are used only when nothing else is.
func incidentMode(p *Incident) detection.Mode {
	keys := make([]detection.Key, 0, len(p.Members))
	for key, member := range p.Members {
		if !isRisk(member) {
			keys = append(keys, key)
		}
	}
	if len(keys) == 0 {
		for key := range p.Members {
			keys = append(keys, key)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Entity != keys[j].Entity {
			return keys[i].Entity.String() < keys[j].Entity.String()
		}
		return keys[i].Reason < keys[j].Reason
	})
	for _, key := range keys {
		if key.Entity == p.Root {
			return p.Members[key].Mode
		}
	}
	if len(keys) > 0 {
		return p.Members[keys[0]].Mode
	}
	return p.Mode
}

// isRisk reports a configuration risk finding.
func isRisk(f detection.Finding) bool {
	return f.Advisory || strings.HasPrefix(f.Reason, reasons.RiskPrefix)
}

// fixOf judges how p ended at now from the change history around its
// root. Without a model the fix is unknown and left empty.
func fixOf(model inventory.HistoryReader, p *Incident, now time.Time) Fix {
	if model == nil {
		return ""
	}
	outcomes := model.ChangeOutcomes(
		p.Root, p.Opened.Add(-inventory.MaxEffectWindow), now)
	for _, outcome := range outcomes {
		if outcome.Outcome == inventory.OutcomeReverted &&
			!outcome.Reverted.Before(p.Opened) {
			return FixRollback
		}
	}
	if nodeReplaced(model, p) {
		return FixNodeReplaced
	}
	return fixByChanges(outcomes, p.Opened)
}

// fixingChange is the latest change around p's root made while it was
// open, other than a scale: the change that most likely fixed it, so the
// resolve can say who fixed it and how. Nil when nothing changed or
// without a model.
func fixingChange(
	model inventory.HistoryReader, p *Incident, now time.Time,
) *inventory.Change {
	if model == nil {
		return nil
	}
	var latest *inventory.Change
	for _, outcome := range model.ChangeOutcomes(
		p.Root, p.Opened.Add(-inventory.MaxEffectWindow), now,
	) {
		for _, change := range outcome.Set.Changes {
			if change.At.Before(p.Opened) || change.At.After(now) ||
				scaleOnly(change) {
				continue
			}
			if latest == nil || change.At.After(latest.At) {
				found := change
				latest = &found
			}
		}
	}
	return latest
}

// scaleOnly reports a change of replica counts or autoscaler bounds
// only. An autoscaler reacts to an incident; it is not its fix.
func scaleOnly(change inventory.Change) bool {
	return change.Classify() == inventory.ClassScale
}

// fixByChanges names the fix from changes made after the incident opened.
func fixByChanges(outcomes []inventory.ChangeOutcome, opened time.Time) Fix {
	fix := FixNone
	for _, outcome := range outcomes {
		for _, change := range outcome.Set.Changes {
			if change.At.Before(opened) || scaleOnly(change) {
				continue
			}
			if change.Classify() == inventory.ClassConfig {
				return FixConfig
			}
			fix = FixChange
		}
	}
	return fix
}

// nodeReplaced reports a node root that failed for real and is gone, or
// was followed by a node added while the incident was open. A drain is
// maintenance, and a node that joined somewhere else says nothing about
// this one, so neither counts.
func nodeReplaced(model inventory.HistoryReader, p *Incident) bool {
	if p.Root.Kind != kube.KindNode || !hadNodeFailure(p) {
		return false
	}
	if !model.Exists(p.Root) {
		return true
	}
	for _, set := range model.RecentChangeSets(p.Opened) {
		for _, change := range set.Changes {
			if change.Classify() == inventory.ClassNodeAdded &&
				!change.At.Before(p.Opened) &&
				sameNodeGroup(model, p.Root, change.Entity) {
				return true
			}
		}
	}
	return false
}

// hadNodeFailure reports that the node had a failing finding of its own
// other than being drained: members now, or reasons seen before they
// cleared.
func hadNodeFailure(p *Incident) bool {
	for key, s := range p.Members {
		if key.Entity == p.Root && !s.Advisory &&
			key.Reason != reasons.NodeDraining {
			return true
		}
	}
	for reason := range p.rootReasons {
		if reason != reasons.NodeDraining {
			return true
		}
	}
	return false
}

// sameNodeGroup reports whether an added node could replace the root:
// it shares the root's node pool or zone. When the root has neither (or
// is gone and cannot be asked), nothing says otherwise, so it matches.
func sameNodeGroup(
	model inventory.HistoryReader, root, added inventory.EntityID,
) bool {
	known := false
	for _, group := range []inventory.Kind{kube.KindNodePool, kube.KindZone} {
		want := relatedOfKind(model, root, group)
		if len(want) == 0 {
			continue
		}
		known = true
		for _, id := range relatedOfKind(model, added, group) {
			if slices.Contains(want, id) {
				return true
			}
		}
	}
	return !known
}

// relatedOfKind lists what id is part of, of one kind.
func relatedOfKind(
	model inventory.Reader, id inventory.EntityID, kind inventory.Kind,
) []inventory.EntityID {
	var out []inventory.EntityID
	for _, target := range model.Related(
		id, inventory.PartOf, inventory.Outgoing) {
		if target.Kind == kind {
			out = append(out, target)
		}
	}
	return out
}
