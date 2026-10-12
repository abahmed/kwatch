package compose

import (
	"strconv"

	"github.com/abahmed/kwatch/internal/inventory"
)

// replicasPath is the field a scale changes.
const replicasPath = "spec.replicas"

// scaleOnly reports a change that touched nothing but the replica
// count: someone or something scaled the workload, which is not a
// release of new code or configuration.
func scaleOnly(change inventory.Change) bool {
	if change.Created || change.Deleted || len(change.Fields) == 0 {
		return false
	}
	for _, field := range change.Fields {
		if field.Path != replicasPath {
			return false
		}
	}
	return true
}

// scaleNoun names a scale as an event: "scale-up", "scale-down", or
// "scaling" when the counts are not known.
func scaleNoun(change inventory.Change) string {
	before, errBefore := strconv.Atoi(change.Fields[0].Before)
	after, errAfter := strconv.Atoi(change.Fields[0].After)
	switch {
	case errBefore != nil || errAfter != nil || before == after:
		return "scaling"
	case after > before:
		return "scale-up"
	}
	return "scale-down"
}

// scaledText words a scale-only change of the root's own workload: "the
// replicas were scaled from 2 to 5 at 10:04".
func scaledText(change inventory.Change) string {
	return "the replicas were scaled" + changeValues(change) + " at " +
		clock(change.At)
}
