package compose

import (
	"github.com/abahmed/kwatch/internal/rootcause"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// modalRows blame a Secret or ConfigMap that is missing or was edited.
// What is wrong with it depends on the mode that matched, so the words
// follow the mode, not the row.
var modalRows = map[string]bool{
	"config-missing-or-changed":        true,
	"configmap-missing-or-changed":     true,
	"run-configmap-missing-or-changed": true,
	"run-secret-missing-or-changed":    true,
}

// modalWords say what is wrong with the object a modal row blames. A
// missing or deleted object "does not exist"; an edited one "changed
// shortly before", never "does not exist", whether or not the change
// itself is attached. A cause with no mode (a record from before modes
// were kept) keeps the row's table words.
func modalWords(cause *rootcause.CauseRecord) (string, bool) {
	if !modalRows[cause.Rule] {
		return "", false
	}
	switch {
	case cause.Mode.Within(explain.ModeMissing),
		cause.Mode.Within(explain.ModeDeleted):
		return "does not exist", true
	case cause.Mode.Within(explain.ModeChanged):
		return "changed shortly before", true
	}
	return "", false
}
