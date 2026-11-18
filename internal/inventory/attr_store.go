package inventory

import (
	"slices"
	"strings"
	"time"
	"unique"
)

// attrEntry is one stored attribute and the source that last set it. It
// is laid out by hand because a large cluster keeps millions of them:
// the name is an interned handle (a few dozen names repeat on every
// pod), the value is stored field by field so its kind shares a word
// with the source index, and flips, which almost no attribute has, sit
// behind a pointer that stays nil until the first flip. Since and
// Updated stay full times so readers get back exactly what was set,
// monotonic reading and location included.
type attrEntry struct {
	name    unique.Handle[string]
	text    string
	num     float64
	since   time.Time
	updated time.Time
	flips   *[]time.Time
	source  sourceID
	kind    valueKind
}

// sourceID names an observation source within one Model.
type sourceID uint32

// sourceTable gives each observation source a small ID. Sources are
// names fixed in code (a dozen or so), so the table stays that small;
// IDs are never reused within a Model.
type sourceTable map[string]sourceID

// id returns the ID of source, assigning the next one on first use.
func (t *sourceTable) id(source string) sourceID {
	if id, ok := (*t)[source]; ok {
		return id
	}
	if *t == nil {
		*t = make(sourceTable)
	}
	id := sourceID(len(*t))
	(*t)[source] = id
	return id
}

func (e *attrEntry) value() Value {
	return Value{kind: e.kind, text: e.text, num: e.num}
}

// attribute rebuilds the public attribute. Flips share the stored
// slice, as they did before: withFlip copies on every change.
func (e *attrEntry) attribute() Attribute {
	out := Attribute{
		Value: e.value(), Since: e.since, Updated: e.updated,
	}
	if e.flips != nil {
		out.Flips = *e.flips
	}
	return out
}

// reset stores a new value first seen at at, with flips (nil for none).
func (e *attrEntry) reset(value Value, at time.Time, flips []time.Time) {
	e.kind, e.text, e.num = value.kind, value.text, value.num
	e.since, e.updated = at, at
	e.flips = nil
	if flips != nil {
		e.flips = &flips
	}
}

// A record keeps its attributes in a slice sorted by name instead of a
// map. A pod has dozens of them, and a map costs about twice as much per
// attribute as a slice entry, which adds up over thousands of pods.

// findAttr returns where name is, or where it would be inserted.
func (r *record) findAttr(name string) (int, bool) {
	return slices.BinarySearchFunc(r.attrs, name,
		func(e attrEntry, name string) int {
			return strings.Compare(e.name.Value(), name)
		})
}

// setAttribute records value for name, set by source and observed at
// at. A value that did not change only refreshes Updated; a boolean that
// did change adds a flip.
func (r *record) setAttribute(
	name string, source sourceID, value Value, at time.Time,
) {
	i, found := r.findAttr(name)
	if !found {
		entry := attrEntry{name: unique.Make(name), source: source}
		entry.reset(value, at, nil)
		r.attrs = slices.Insert(r.attrs, i, entry)
		return
	}
	entry := &r.attrs[i]
	entry.source = source
	if entry.value().Equal(value) {
		entry.updated = at
		return
	}
	var flips []time.Time
	if _, isBool := value.AsBool(); isBool {
		flips = withFlip(entry.attribute().Flips, at)
	}
	entry.reset(value, at, flips)
}

// dropAttributesFrom removes the attributes source set earlier and no
// longer reports, those not in keep.
func (r *record) dropAttributesFrom(
	source sourceID, keep map[string]Value,
) {
	r.attrs = slices.DeleteFunc(r.attrs, func(e attrEntry) bool {
		_, still := keep[e.name.Value()]
		return e.source == source && !still
	})
}

// snapshot returns a detached copy of the entity with its attributes.
func (r *record) snapshot() Entity {
	out := r.entity
	out.Attributes = make(map[string]Attribute, len(r.attrs))
	for i := range r.attrs {
		out.Attributes[r.attrs[i].name.Value()] = r.attrs[i].attribute()
	}
	return out
}

// attribute returns one stored attribute.
func (r *record) attribute(name string) (Attribute, bool) {
	if i, found := r.findAttr(name); found {
		return r.attrs[i].attribute(), true
	}
	return Attribute{}, false
}
