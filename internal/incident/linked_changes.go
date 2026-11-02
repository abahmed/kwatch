package incident

import (
	"cmp"
	"slices"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/redact"
)

// Linked change bounds: the changes kept per incident, the fields kept
// per change and the bytes kept per value.
const (
	maxLinkedChanges = 12
	maxChangeFields  = 6
	maxChangeValue   = 160
)

// linkChanges copies the changes the history has around p into its
// journal: those made in the RecentWindow before its failures began and
// those since, ranked as the dashboard ranks them (ChangesBetween). The
// change history keeps about two hours; the incident keeps these.
func (m *Manager) linkChanges(p *Incident, now time.Time) {
	if m.model == nil {
		return
	}
	onset := p.Onset()
	if onset.IsZero() {
		return
	}
	found := ChangesBetween(m.model, p.Root,
		onset.Add(-RecentWindow), onset, maxLinkedChanges)
	if now.After(onset) {
		found = append(found, ChangesBetween(m.model, p.Root,
			onset.Add(time.Nanosecond), now, maxLinkedChanges)...)
	}
	if len(found) > 0 {
		p.Journal.Changes = mergeLinked(p.Journal.Changes, found, onset)
	}
}

// mergeLinked adds the fresh changes that are not held yet. Changes
// before onset come first, then those during it, each newest first;
// beyond maxLinkedChanges the oldest during it go, so what preceded the
// failure is kept longest.
func mergeLinked(
	held, fresh []inventory.Change, onset time.Time,
) []inventory.Change {
	out := append([]inventory.Change(nil), held...)
	for _, change := range fresh {
		if !slices.ContainsFunc(out, sameChange(change)) {
			out = append(out, SafeChange(change))
		}
	}
	slices.SortStableFunc(out, func(a, b inventory.Change) int {
		aAfter, bAfter := a.At.After(onset), b.At.After(onset)
		if aAfter != bAfter {
			return boolOrder(aAfter, bAfter)
		}
		return cmp.Or(b.At.Compare(a.At),
			strings.Compare(a.Entity.String(), b.Entity.String()))
	})
	if len(out) > maxLinkedChanges {
		out = out[:maxLinkedChanges]
	}
	return out
}

// boolOrder orders false before true.
func boolOrder(a, b bool) int {
	switch {
	case a == b:
		return 0
	case a:
		return 1
	}
	return -1
}

// sameChange matches the change of the same object at the same time.
func sameChange(c inventory.Change) func(inventory.Change) bool {
	return func(o inventory.Change) bool {
		return o.Entity == c.Entity && o.At.Equal(c.At)
	}
}

// SafeChange is c as kwatch keeps or serves it: free text redacted and
// bounded, at most maxChangeFields fields, and the data of a Secret or
// a ConfigMap named by its keys only, never its values. The source
// already writes data edits as "added", "changed" or "removed"; this
// holds that rule whatever the source wrote, so a data field always
// carries one of the three.
func SafeChange(c inventory.Change) inventory.Change {
	out := c
	out.Actor = boundText(c.Actor)
	out.App = boundText(c.App)
	out.Cause = boundText(c.Cause)
	out.Revision = boundText(c.Revision)
	fields := c.Fields[:min(len(c.Fields), maxChangeFields)]
	out.Fields = make([]inventory.FieldChange, 0, len(fields))
	for _, f := range fields {
		out.Fields = append(out.Fields, safeField(c.Entity.Kind, f))
	}
	if len(out.Fields) == 0 {
		out.Fields = nil
	}
	return out
}

// safeField bounds one field change and keeps only the edit marker of
// a data key.
func safeField(
	kind inventory.Kind, f inventory.FieldChange,
) inventory.FieldChange {
	out := inventory.FieldChange{Path: truncateRunes(f.Path, maxChangeValue)}
	if dataKey(kind, f.Path) {
		out.After = dataMarker(f)
		return out
	}
	out.Before, out.After = boundText(f.Before), boundText(f.After)
	return out
}

// dataKey reports a path that holds a Secret's or a ConfigMap's data,
// or any object's data keys.
func dataKey(kind inventory.Kind, path string) bool {
	if kind == kube.KindSecret || kind == kube.KindConfigMap {
		return true
	}
	for _, prefix := range []string{"data.", "stringData.", "binaryData."} {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

// dataMarker keeps the source's edit word. Any other value is replaced
// by the word its shape implies, never shown: a value only after is
// "added", one only before "removed", anything else "changed".
func dataMarker(f inventory.FieldChange) string {
	switch f.After {
	case "added", "changed", "removed":
		return f.After
	}
	switch {
	case f.Before == "" && f.After != "":
		return "added"
	case f.Before != "" && f.After == "":
		return "removed"
	}
	return "changed"
}

// boundText redacts text and cuts it to maxChangeValue bytes.
func boundText(text string) string {
	return truncateRunes(redact.Evidence(text), maxChangeValue)
}
