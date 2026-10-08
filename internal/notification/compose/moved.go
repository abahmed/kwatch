package compose

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
)

// movedNote is the close of an incident whose failures another incident
// took over, in the words of the thread that reads it: no incident ids,
// no claim that anything is healthy.
func movedNote(f caseFacts) []sentence {
	p := f.p
	if p.SupersededRoot == (inventory.EntityID{}) {
		return []sentence{{part: partLead,
			text: "Moved: the failures of " + f.leadName(p.Root) +
				" are now part of another incident and are not resolved."}}
	}
	return []sentence{{part: partLead, text: "Moved: this is now part of " +
		"the " + shortName(p.SupersededRoot) + movedNoun(p.Mode) +
		movedPlace(p.SupersededRoot) + f.clusterTag() + "."}}
}

// movedNoun words what failed: a crash, or else a failure.
func movedNoun(mode detection.Mode) string {
	if mode == detection.ModeCrashLoop {
		return " crash"
	}
	return " failure"
}

// movedPlace is " in staging" for an object of a namespace.
func movedPlace(id inventory.EntityID) string {
	if id.Namespace == "" {
		return ""
	}
	return " in " + id.Namespace
}
