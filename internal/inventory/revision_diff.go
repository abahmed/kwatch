package inventory

import "strings"

// templateFieldPaths start the paths of a pod template edit: the ones a
// new revision carries. Replica counts and the like are not part of one.
var templateFieldPaths = []string{"containers[", "volumes[", "template."}

// IsTemplateField reports a field path inside a pod template.
func IsTemplateField(path string) bool {
	for _, prefix := range templateFieldPaths {
		if strings.HasPrefix(path, prefix) {
			return true
		}
	}
	return false
}

// RevisionDiff folds the pod template edits of changes (oldest first)
// into what differs between the revision before them and the one after:
// per path the first old value and the last new value. A path edited
// back to where it began drops out. The result is ranked, likeliest
// culprit first (see RankFields).
func RevisionDiff(changes []Change) []FieldChange {
	var order []string
	merged := map[string]FieldChange{}
	for _, change := range changes {
		for _, field := range change.Fields {
			if !IsTemplateField(field.Path) {
				continue
			}
			if first, ok := merged[field.Path]; ok {
				first.After = field.After
				merged[field.Path] = first
				continue
			}
			order = append(order, field.Path)
			merged[field.Path] = field
		}
	}
	var out []FieldChange
	for _, path := range order {
		if field := merged[path]; field.Before != field.After {
			out = append(out, field)
		}
	}
	return RankFields(out)
}
