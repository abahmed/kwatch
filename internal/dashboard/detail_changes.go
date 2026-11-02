package dashboard

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/redact"
)

// maxChanges bounds the changes the page lists; changeScan is how many
// it asks for, to count the ones left out.
const (
	maxChanges = 8
	changeScan = 64
)

// changesOf lists the changes people made around p from RecentWindow
// before its failures began until it resolved (or now): those before
// first, each part most relevant first, and the change kwatch judged
// most likely fixed it. cut counts the changes left out.
func changesOf(
	model inventory.HistoryReader, p incident.Incident, now time.Time,
) (list []ChangeItem, fixedBy *ChangeItem, cut int) {
	onset := p.Onset()
	end := p.Resolved
	if end.IsZero() {
		end = now
	}
	if p.FixedBy != nil {
		item := toChangeItem(*p.FixedBy, onset)
		item.Fixing = true
		fixedBy = &item
	}
	if onset.IsZero() || end.IsZero() {
		return nil, fixedBy, 0
	}
	// Before and during are read apart: each keeps the newest change
	// per object, and a fix attempt during the incident must not hide
	// the change that came before it.
	found := incident.ChangesBetween(model, p.Root,
		onset.Add(-incident.RecentWindow), onset, changeScan)
	if end.After(onset) {
		found = append(found, incident.ChangesBetween(model, p.Root,
			onset.Add(time.Nanosecond), end, changeScan)...)
	}
	found = withStored(p.Journal.Changes, found, onset)
	if len(found) > maxChanges {
		cut = len(found) - maxChanges
		found = found[:maxChanges]
	}
	for _, change := range found {
		item := toChangeItem(change, onset)
		item.Fixing = p.FixedBy != nil &&
			change.Entity == p.FixedBy.Entity &&
			change.At.Equal(p.FixedBy.At)
		item.Cause = blamed(p, change)
		list = append(list, item)
	}
	return list, fixedBy, cut
}

// blamed says whether change is the one p's likely cause blames.
func blamed(p incident.Incident, change inventory.Change) bool {
	if p.Cause == nil || p.Cause.Change == nil {
		return false
	}
	c := p.Cause.Change
	return change.Entity == c.Entity && change.At.Equal(c.At)
}

// withStored puts the changes p stored first, since they outlive the
// change history, then the live ones it did not store; those before
// onset come before those during it, each part in that order.
func withStored(
	stored, live []inventory.Change, onset time.Time,
) []inventory.Change {
	out := make([]inventory.Change, 0, len(stored)+len(live))
	for _, during := range []bool{false, true} {
		for _, list := range [][]inventory.Change{stored, live} {
			for _, change := range list {
				if change.At.After(onset) == during &&
					!slices.ContainsFunc(out, sameChange(change)) {
					out = append(out, change)
				}
			}
		}
	}
	return out
}

// sameChange matches the change of the same object at the same time.
func sameChange(c inventory.Change) func(inventory.Change) bool {
	return func(o inventory.Change) bool {
		return o.Entity == c.Entity && o.At.Equal(c.At)
	}
}

// toChangeItem describes change for the page; before says whether it
// came before onset.
func toChangeItem(change inventory.Change, onset time.Time) ChangeItem {
	when := "during"
	if onset.IsZero() || !change.At.After(onset) {
		when = "before"
	}
	return ChangeItem{
		At: change.At, When: when,
		Kind:      string(change.Entity.Kind),
		Namespace: change.Entity.Namespace,
		Name:      change.Entity.Name,
		Class:     string(change.Classify()),
		Summary:   redact.Evidence(changeSummary(change)),
		Actor:     redact.Evidence(change.Actor),
		App:       redact.Evidence(change.App),
		Note:      redact.Evidence(change.Cause),
		Fields:    fieldDiffs(change),
	}
}

// fieldDiffs are change's fields as the diff view shows them: bounded,
// redacted, and a data key without its values (incident.SafeChange).
func fieldDiffs(change inventory.Change) []FieldDiff {
	var out []FieldDiff
	for _, f := range incident.SafeChange(change).Fields {
		out = append(out, FieldDiff{Path: f.Path,
			Before: redact.Evidence(f.Before),
			After:  redact.Evidence(f.After)})
	}
	return out
}

// changeSummary says what a change did in a few words. Only image
// references and replica counts are quoted: a data key's value may be
// a secret, so a key edit names the key alone.
func changeSummary(change inventory.Change) string {
	switch {
	case change.Created:
		return "created"
	case change.Deleted:
		return "deleted"
	case len(change.Fields) == 0:
		return "changed"
	}
	text := fieldSummary(change.Fields[0])
	if extra := len(change.Fields) - 1; extra > 0 {
		text += " (and " + strconv.Itoa(extra) + " more)"
	}
	return text
}

func fieldSummary(f inventory.FieldChange) string {
	if key, ok := strings.CutPrefix(f.Path, "data."); ok {
		return "key " + key + " changed"
	}
	switch {
	case strings.HasSuffix(f.Path, ".image"):
		return "image " + fromTo(imageName(f.Before), imageName(f.After))
	case strings.HasSuffix(f.Path, "replicas"):
		return "replicas " + fromTo(f.Before, f.After)
	}
	return f.Path + " changed"
}

// fromTo is "a → b", or "set to b" when there was no value before.
func fromTo(before, after string) string {
	if before == "" {
		return "set to " + after
	}
	if after == "" {
		return before + " removed"
	}
	return before + " → " + after
}

// imageName is an image reference without its registry and path:
// "registry.io/team/api:1.2" is "api:1.2". A digest is kept short.
func imageName(ref string) string {
	if i := strings.LastIndex(ref, "/"); i >= 0 {
		ref = ref[i+1:]
	}
	if name, digest, ok := strings.Cut(ref, "@sha256:"); ok &&
		len(digest) > 12 {
		return name + "@sha256:" + digest[:12]
	}
	return ref
}
