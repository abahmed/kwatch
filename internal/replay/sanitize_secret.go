package replay

import (
	"strings"

	"github.com/abahmed/kwatch/internal/inventory"
)

// secretKind is the entity kind of Secrets. Like imageKind it is spelled
// here so replay stays independent of the Kubernetes schema package.
const secretKind inventory.Kind = "secret"

// hideSecret hides, in a copy of out, everything textual a Secret's
// observation carries. kwatch records only key names, digests and types
// of a Secret, never values, but a key name can still say too much, and a
// short digest or type could pass for a reason code. Numbers, booleans
// and times stay, so replays still see that a Secret changed and when.
func (s *Sanitizer) hideSecret(
	out inventory.Observation,
) inventory.Observation {
	if out.Entity.Kind != secretKind &&
		out.Change.Entity.Kind != secretKind {
		return out
	}
	if out.Entity.Kind == secretKind && out.Attributes != nil {
		hidden := make(map[string]inventory.Value, len(out.Attributes))
		for name, value := range out.Attributes {
			hidden[name] = s.hideText(value)
		}
		out.Attributes = hidden
	}
	if out.Change.Entity.Kind == secretKind {
		fields := make([]inventory.FieldChange, 0, len(out.Change.Fields))
		for _, field := range out.Change.Fields {
			fields = append(fields, s.hideField(field))
		}
		out.Change.Fields = fields
	}
	return out
}

// hideText hides a text value, hashing it so equal texts stay equal.
func (s *Sanitizer) hideText(value inventory.Value) inventory.Value {
	if _, isNumber := value.AsNumber(); isNumber {
		return value
	}
	if _, isBool := value.AsBool(); isBool || isTime(value) ||
		value.IsZero() {
		return value
	}
	return inventory.Text(s.optional("secret", value.AsText()))
}

// hideField hides a Secret's field change: the key named in the path and
// the before and after texts, which differ exactly when the originals do.
func (s *Sanitizer) hideField(f inventory.FieldChange) inventory.FieldChange {
	path := f.Path
	if rest, ok := strings.CutPrefix(path, "data."); ok {
		path = "data." + s.optional("key", rest)
	} else {
		path = s.optional("key", path)
	}
	return inventory.FieldChange{
		Path:   path,
		Before: s.optional("secret", f.Before),
		After:  s.optional("secret", f.After),
	}
}
