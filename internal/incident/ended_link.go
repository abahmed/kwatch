package incident

import "github.com/abahmed/kwatch/internal/inventory"

// endedLinkVerbs say what the end of a link of each relation type
// means for the object that held it.
var endedLinkVerbs = map[inventory.RelationType]string{
	inventory.RoutesTo:   "stopped routing to",
	inventory.Selects:    "stopped covering",
	inventory.References: "no longer uses",
	inventory.Mounts:     "no longer mounts",
	inventory.Serves:     "is no longer served by",
	inventory.Calls:      "no longer calls",
}

// EndedLink says what a change that records the end of a link did, for
// a message or a page: the words between the two objects ("stopped
// routing to") and the object the link pointed to. ok is false for any
// other change.
func EndedLink(
	change inventory.Change,
) (words string, target inventory.EntityID, ok bool) {
	rel, target, ok := change.RemovedLink()
	if !ok {
		return "", inventory.EntityID{}, false
	}
	words, known := endedLinkVerbs[rel]
	if !known {
		words = "no longer relates to"
	}
	return words, target, true
}
