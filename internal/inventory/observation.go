package inventory

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"
)

// ObservationKind selects how an Observation updates the model.
type ObservationKind uint8

const (
	// Observed records that an entity exists and sets its attributes.
	Observed ObservationKind = iota + 1
	// Related replaces one relation type's targets for an entity.
	Related
	// Changed appends a meaningful change to the entity's history.
	Changed
	// Gone removes an entity and every relation touching it.
	Gone
	// Noted records an occurrence about an entity (an event). The entity
	// need not be observed; notes about unknown entities are kept too.
	Noted
)

// Observation is the only input the model accepts. Every source, from
// Kubernetes informers to a future node agent, speaks in observations.
type Observation struct {
	Kind   ObservationKind
	Source string
	At     time.Time
	Entity EntityID
	UID    string

	// Attributes are set by Observed observations. An attribute absent from a
	// later Observed observation from the same source is removed.
	Attributes map[string]Value

	// Relation and Targets are set by Related observations. The observation
	// replaces the entity's previous targets of that type from the same source, so
	// a source never has to diff edges itself.
	Relation RelationType
	Targets  []EntityID

	// Change is set by Changed observations.
	Change Change

	// Note is set by Noted observations. A note with the same source and reason
	// replaces the previous one, so repeated events do not fill the ring.
	Note Note
}

// observationKindNames are the stable wire names of observation kinds.
// They appear in recorded observation logs; never rename one.
var observationKindNames = [...]string{
	Observed: "observed",
	Related:  "related",
	Changed:  "changed",
	Gone:     "gone",
	Noted:    "noted",
}

// String returns the stable wire name, or "unknown".
func (k ObservationKind) String() string {
	if int(k) < len(observationKindNames) && observationKindNames[k] != "" {
		return observationKindNames[k]
	}
	return "unknown"
}

// MarshalText encodes the kind by its stable name.
func (k ObservationKind) MarshalText() ([]byte, error) {
	name := k.String()
	if name == "unknown" {
		return nil, fmt.Errorf("inventory: unknown observation kind %d", k)
	}
	return []byte(name), nil
}

// UnmarshalText decodes a stable kind name.
func (k *ObservationKind) UnmarshalText(text []byte) error {
	for kind, name := range observationKindNames {
		if name != "" && name == string(text) {
			*k = ObservationKind(kind)
			return nil
		}
	}
	return fmt.Errorf("inventory: unknown observation kind %q", text)
}

// The wire types below give observations a stable lower-case JSON shape
// for recorded logs. They are separate from EntityID, Change and Note so
// the default encoding of those types, which persisted incident records
// already use, never changes.
type observationWire struct {
	Kind       ObservationKind  `json:"kind"`
	Source     string           `json:"source,omitempty"`
	At         *time.Time       `json:"at,omitempty"`
	Entity     entityWire       `json:"entity"`
	UID        string           `json:"uid,omitempty"`
	Attributes map[string]Value `json:"attributes,omitempty"`
	Relation   RelationType     `json:"relation,omitempty"`
	Targets    []entityWire     `json:"targets,omitempty"`
	Change     *changeWire      `json:"change,omitempty"`
	Note       *noteWire        `json:"note,omitempty"`
}

// entityWire omits an empty group, so logs recorded before groups
// existed decode into the built-in group unchanged.
type entityWire struct {
	Group     string `json:"group,omitempty"`
	Kind      Kind   `json:"kind"`
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name"`
}

type changeWire struct {
	Entity   *entityWire       `json:"entity,omitempty"`
	At       *time.Time        `json:"at,omitempty"`
	Observed *time.Time        `json:"observed,omitempty"`
	Actor    string            `json:"actor,omitempty"`
	App      string            `json:"app,omitempty"`
	Revision string            `json:"revision,omitempty"`
	Created  bool              `json:"created,omitempty"`
	Deleted  bool              `json:"deleted,omitempty"`
	Fields   []fieldChangeWire `json:"fields,omitempty"`
}

type fieldChangeWire struct {
	Path   string `json:"path"`
	Before string `json:"before,omitempty"`
	After  string `json:"after,omitempty"`
}

type noteWire struct {
	At        *time.Time `json:"at,omitempty"`
	FirstSeen *time.Time `json:"first_seen,omitempty"`
	Source    string     `json:"source,omitempty"`
	Reason    string     `json:"reason,omitempty"`
	Message   string     `json:"message,omitempty"`
	Count     int        `json:"count,omitempty"`
	Warning   bool       `json:"warning,omitempty"`
}

// MarshalJSON encodes the observation in the stable recorded-log shape.
func (o Observation) MarshalJSON() ([]byte, error) {
	wire := observationWire{
		Kind: o.Kind, Source: o.Source, At: wireTime(o.At),
		Entity: toEntityWire(o.Entity), UID: o.UID,
		Attributes: o.Attributes, Relation: o.Relation,
	}
	for _, target := range o.Targets {
		wire.Targets = append(wire.Targets, toEntityWire(target))
	}
	if !isZeroChange(o.Change) {
		wire.Change = toChangeWire(o.Change)
	}
	if o.Note != (Note{}) {
		wire.Note = &noteWire{
			At: wireTime(o.Note.At), FirstSeen: wireTime(o.Note.FirstSeen),
			Source: o.Note.Source,
			Reason: o.Note.Reason, Message: o.Note.Message,
			Count: o.Note.Count, Warning: o.Note.Warning,
		}
	}
	return json.Marshal(wire)
}

// UnmarshalJSON decodes the stable recorded-log shape.
func (o *Observation) UnmarshalJSON(data []byte) error {
	var wire observationWire
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	if wire.Kind == 0 {
		return errors.New("inventory: observation without kind")
	}
	out := Observation{
		Kind: wire.Kind, Source: wire.Source, At: fromWireTime(wire.At),
		Entity: wire.Entity.id(), UID: wire.UID,
		Attributes: wire.Attributes, Relation: wire.Relation,
	}
	for _, target := range wire.Targets {
		out.Targets = append(out.Targets, target.id())
	}
	if wire.Change != nil {
		out.Change = wire.Change.change()
	}
	if wire.Note != nil {
		out.Note = Note{
			At:        fromWireTime(wire.Note.At),
			FirstSeen: fromWireTime(wire.Note.FirstSeen),
			Source:    wire.Note.Source,
			Reason:    wire.Note.Reason, Message: wire.Note.Message,
			Count: wire.Note.Count, Warning: wire.Note.Warning,
		}
	}
	*o = out
	return nil
}

func toEntityWire(id EntityID) entityWire {
	return entityWire{
		Group: id.Group, Kind: id.Kind, Namespace: id.Namespace, Name: id.Name,
	}
}

func (w entityWire) id() EntityID {
	return NewEntityID(w.Group, w.Kind, w.Namespace, w.Name)
}

func isZeroChange(c Change) bool {
	return c.Entity.IsZero() && c.At.IsZero() && c.Observed.IsZero() &&
		c.Actor == "" && c.App == "" && c.Revision == "" &&
		!c.Created && !c.Deleted && len(c.Fields) == 0
}

func toChangeWire(c Change) *changeWire {
	wire := &changeWire{
		At: wireTime(c.At), Observed: wireTime(c.Observed),
		Actor: c.Actor, App: c.App, Revision: c.Revision,
		Created: c.Created, Deleted: c.Deleted,
	}
	if !c.Entity.IsZero() {
		entity := toEntityWire(c.Entity)
		wire.Entity = &entity
	}
	for _, field := range c.Fields {
		wire.Fields = append(wire.Fields, fieldChangeWire(field))
	}
	return wire
}

func (w *changeWire) change() Change {
	out := Change{
		At: fromWireTime(w.At), Observed: fromWireTime(w.Observed),
		Actor: w.Actor, App: w.App, Revision: w.Revision,
		Created: w.Created, Deleted: w.Deleted,
	}
	if w.Entity != nil {
		out.Entity = w.Entity.id()
	}
	for _, field := range w.Fields {
		out.Fields = append(out.Fields, FieldChange(field))
	}
	return out
}

// wireTime encodes a timestamp in UTC; the zero time is omitted.
func wireTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	utc := t.UTC()
	return &utc
}

func fromWireTime(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return t.UTC()
}

// valueWire is the stable JSON shape of a Value: exactly one field is set,
// and the unset value encodes as null.
type valueWire struct {
	Text   *string  `json:"text,omitempty"`
	Number *float64 `json:"number,omitempty"`
	Bool   *bool    `json:"bool,omitempty"`
	Time   string   `json:"time,omitempty"`
}

// MarshalJSON encodes the value with its type, so a round trip keeps it.
func (v Value) MarshalJSON() ([]byte, error) {
	var wire valueWire
	switch v.kind {
	case valueNone:
		return []byte("null"), nil
	case valueText:
		wire.Text = &v.text
	case valueNumber:
		if math.IsNaN(v.num) || math.IsInf(v.num, 0) {
			return nil, fmt.Errorf("inventory: value %v is not finite", v.num)
		}
		wire.Number = &v.num
	case valueBool:
		flag := v.num != 0
		wire.Bool = &flag
	case valueTime:
		wire.Time = v.AsText()
	default:
		return nil, fmt.Errorf("inventory: unknown value kind %d", v.kind)
	}
	return json.Marshal(wire)
}

// UnmarshalJSON decodes the typed shape written by MarshalJSON.
func (v *Value) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		*v = Value{}
		return nil
	}
	var wire valueWire
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	set := 0
	for _, present := range []bool{
		wire.Text != nil, wire.Number != nil, wire.Bool != nil,
		wire.Time != "",
	} {
		if present {
			set++
		}
	}
	if set != 1 {
		return errors.New("inventory: value must set exactly one type")
	}
	switch {
	case wire.Text != nil:
		*v = Text(*wire.Text)
	case wire.Number != nil:
		*v = Number(*wire.Number)
	case wire.Bool != nil:
		*v = Bool(*wire.Bool)
	default:
		at, err := time.Parse(time.RFC3339, wire.Time)
		if err != nil {
			return fmt.Errorf("inventory: value time: %w", err)
		}
		*v = Time(at)
	}
	return nil
}
