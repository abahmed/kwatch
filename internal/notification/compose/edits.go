package compose

import (
	"regexp"
	"strings"

	"github.com/abahmed/kwatch/internal/inventory"
)

// arrow joins the old and new value of an edit: "memory limit 512Mi →
// 256Mi".
const arrow = " → "

// maxEdits bounds the edits one sentence lists.
const maxEdits = 3

var (
	containerPath = regexp.MustCompile(
		`^containers\[[^\]]*\]\.(.+)$`)
	resourcePath = regexp.MustCompile(
		`^resources\.(limits|requests)\.(\w+)$`)
	volumePath = regexp.MustCompile(`^volumes\[([^\]]*)\]$`)
	// containerAdded is the whole container appearing or disappearing.
	containerAdded = regexp.MustCompile(`^containers\[([^\]]*)\]$`)
)

// editWords says one edit the way a person would: "env DB_HOST db-old →
// db-new", "memory limit 512Mi → 256Mi", "image api:1 → api:2". Edits
// with nothing readable to show (a key of a Secret, a hash) say only what
// changed.
func editWords(field inventory.FieldChange) string {
	label := editLabel(field.Path)
	if key, ok := dataKey(field.Path); ok {
		return "key " + key + " changed"
	}
	if containerAdded.MatchString(field.Path) {
		return label + " " + editValue(field.After+field.Before)
	}
	if opaque(field.Before) && opaque(field.After) ||
		field.Before == "changed" || field.After == "changed" {
		return label + " changed"
	}
	return label + " " + editValue(field.Before) + arrow +
		editValue(field.After)
}

func editValue(value string) string {
	if value == "" {
		return "unset"
	}
	return value
}

// editLabel names the edited field: "env DB_HOST", "memory limit",
// "liveness probe".
func editLabel(path string) string {
	if m := containerAdded.FindStringSubmatch(path); m != nil {
		return "container " + m[1]
	}
	if m := volumePath.FindStringSubmatch(path); m != nil {
		return "volume " + m[1]
	}
	inner := path
	if m := containerPath.FindStringSubmatch(path); m != nil {
		inner = m[1]
	}
	switch {
	case strings.HasPrefix(inner, "env."):
		return "env " + strings.TrimPrefix(inner, "env.")
	case inner == "envFrom":
		return "env source"
	case strings.HasSuffix(inner, "Probe"):
		return strings.TrimSuffix(inner, "Probe") + " probe"
	}
	if m := resourcePath.FindStringSubmatch(inner); m != nil {
		return m[2] + " " + strings.TrimSuffix(m[1], "s")
	}
	return strings.TrimPrefix(fieldNoun(inner), "the ")
}

// editList joins the first maxEdits edits and counts the rest.
func editList(fields []inventory.FieldChange) string {
	var items []string
	for _, field := range fields {
		items = append(items, editWords(field))
	}
	if extra := len(items) - maxEdits; extra > 0 {
		items = append(items[:maxEdits], plural(extra, "more edit"))
	}
	return strings.Join(items, "; ")
}

// readableEdits are the fields of a change that show values a reader can
// use: not hashes, not whole-object digests.
func readableEdits(change inventory.Change) []inventory.FieldChange {
	var out []inventory.FieldChange
	for _, field := range change.Fields {
		if field.Path == "spec.template" || field.Path == "spec" ||
			field.Path == "(more fields)" ||
			(opaque(field.Before) && opaque(field.After)) {
			continue
		}
		out = append(out, field)
	}
	return out
}

// valueEdits are the readable edits of a change that are not config keys:
// the ones worth spelling out next to the object's name.
func valueEdits(change inventory.Change) []inventory.FieldChange {
	var out []inventory.FieldChange
	for _, field := range readableEdits(change) {
		if _, key := dataKey(field.Path); !key {
			out = append(out, field)
		}
	}
	return out
}
