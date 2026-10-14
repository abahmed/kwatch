package inventory

import (
	"slices"
	"strings"
	"time"
)

// attrEntry is one stored attribute and the source that last set it.
type attrEntry struct {
	name   string
	source string
	attr   Attribute
}

// A record keeps its attributes in a slice sorted by name instead of a
// map. A pod has dozens of them, and a map costs about twice as much per
// attribute as a slice entry, which adds up over thousands of pods.

// findAttr returns where name is, or where it would be inserted.
func (r *record) findAttr(name string) (int, bool) {
	return slices.BinarySearchFunc(r.attrs, name,
		func(e attrEntry, name string) int {
			return strings.Compare(e.name, name)
		})
}

// setAttribute records value for name, set by source and observed at
// at. A value that did not change only refreshes Updated; a boolean that
// did change adds a flip.
func (r *record) setAttribute(
	name, source string, value Value, at time.Time,
) {
	i, found := r.findAttr(name)
	if !found {
		next := Attribute{Value: value, Since: at, Updated: at}
		r.attrs = slices.Insert(r.attrs, i,
			attrEntry{name: name, source: source, attr: next})
		return
	}
	entry := &r.attrs[i]
	entry.source = source
	if entry.attr.Value.Equal(value) {
		entry.attr.Updated = at
		return
	}
	next := Attribute{Value: value, Since: at, Updated: at}
	if _, isBool := value.AsBool(); isBool {
		next.Flips = withFlip(entry.attr.Flips, at)
	}
	entry.attr = next
}

// dropAttributesFrom removes the attributes source set earlier and no
// longer reports, those not in keep.
func (r *record) dropAttributesFrom(source string, keep map[string]Value) {
	r.attrs = slices.DeleteFunc(r.attrs, func(e attrEntry) bool {
		_, still := keep[e.name]
		return e.source == source && !still
	})
}

// snapshot returns a detached copy of the entity with its attributes.
func (r *record) snapshot() Entity {
	out := r.entity
	out.Attributes = make(map[string]Attribute, len(r.attrs))
	for _, e := range r.attrs {
		out.Attributes[e.name] = e.attr
	}
	return out
}

// attribute returns one stored attribute.
func (r *record) attribute(name string) (Attribute, bool) {
	if i, found := r.findAttr(name); found {
		return r.attrs[i].attr, true
	}
	return Attribute{}, false
}
