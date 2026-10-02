package incident

import (
	"sort"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
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
	Mode     detection.Mode `json:",omitempty"`
	Opened   time.Time
	Resolved time.Time
	Fix      Fix
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
		Mode: p.Mode, Opened: p.Opened, Resolved: p.Resolved, Fix: p.Fix,
	}
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
// member by key when the root has none.
func incidentMode(p *Incident) detection.Mode {
	keys := make([]detection.Key, 0, len(p.Members))
	for key := range p.Members {
		keys = append(keys, key)
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

// nodeReplaced reports a node root that is gone, or a node added while
// the incident was open.
func nodeReplaced(model inventory.HistoryReader, p *Incident) bool {
	if p.Root.Kind != kube.KindNode {
		return false
	}
	if !model.Exists(p.Root) {
		return true
	}
	for _, set := range model.RecentChangeSets(p.Opened) {
		for _, change := range set.Changes {
			if change.Classify() == inventory.ClassNodeAdded &&
				!change.At.Before(p.Opened) {
				return true
			}
		}
	}
	return false
}
