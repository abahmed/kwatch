package inventory

import "strings"

// A link that ends is a change too: a Service whose selector stops
// matching a workload, a workload that no longer mounts a ConfigMap. The
// model keeps only the relations that hold now, so such a change is
// recorded on the object that held the link, as one field whose path
// names the relation and whose Before is the object it pointed to.
// Sources decide which links matter (see kube.LinkJournal).

// linkPathPrefix starts the field path of a link that ended.
const linkPathPrefix = "link."

// LinkRemoved is the field of a change that records that the link of
// type relation to target ended.
func LinkRemoved(relation RelationType, target EntityID) FieldChange {
	return FieldChange{
		Path: linkPathPrefix + string(relation), Before: target.String(),
	}
}

// RemovedLink reports the relation type and target of a change that
// records a link that ended (LinkRemoved). ok is false for any other
// change.
func (c Change) RemovedLink() (RelationType, EntityID, bool) {
	if len(c.Fields) != 1 || c.Created || c.Deleted {
		return "", EntityID{}, false
	}
	field := c.Fields[0]
	relation, ok := strings.CutPrefix(field.Path, linkPathPrefix)
	if !ok || relation == "" || field.After != "" {
		return "", EntityID{}, false
	}
	target, ok := ParseEntityID(field.Before)
	if !ok {
		return "", EntityID{}, false
	}
	return RelationType(relation), target, true
}

// SameChange reports a and b as the same change: the same object
// changed at the same time, and both or neither the end of the same
// link. A link that ended with an edit of its object is told apart
// from the edit.
func SameChange(a, b Change) bool {
	if a.Entity != b.Entity || !a.At.Equal(b.At) {
		return false
	}
	ra, ta, la := a.RemovedLink()
	rb, tb, lb := b.RemovedLink()
	return la == lb && ra == rb && ta == tb
}
