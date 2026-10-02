package replay

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"

	"github.com/abahmed/kwatch/internal/inventory"
)

// SanitizeOptions tune a Sanitizer. Salt is required; with only a salt
// everything is hidden except reason codes, numbers, booleans and times.
type SanitizeOptions struct {
	// Salt keys the pseudonyms and must not be empty. Logs sanitized with
	// the same salt use the same pseudonyms; keep it secret when the
	// names must not be guessed.
	Salt string
	// KeepText names attributes whose text is kept verbatim.
	KeepText []string
	// KeepMessages keeps note messages. They are free text and dropped by
	// default.
	KeepMessages bool
}

// Sanitizer replaces identifying data in observations with stable
// pseudonyms so a log from a real cluster can be shared or committed.
//
// Names, namespaces, UIDs, actors, GitOps applications, note sources and
// images become hash-based pseudonyms:
// the same input always yields the same pseudonym, so relations, owner
// chains and label selectors still line up after sanitizing. Label keys
// are kept and label values are hashed. Kinds, relation types, sources,
// attribute names, reason codes, numbers, booleans and times are kept.
// Free text (messages, errors, anything containing spaces) is dropped.
type Sanitizer struct {
	salt         string
	keep         map[string]bool
	keepMessages bool
}

// ErrNoSalt is returned by NewSanitizer without a salt: unsalted
// pseudonyms of common names ("default", "kube-system") are easy to
// reverse by hashing guesses.
var ErrNoSalt = errors.New("replay: sanitizer needs a salt")

// NewSanitizer builds a sanitizer. It requires a salt.
func NewSanitizer(opts SanitizeOptions) (*Sanitizer, error) {
	if opts.Salt == "" {
		return nil, ErrNoSalt
	}
	keep := make(map[string]bool, len(opts.KeepText))
	for _, name := range opts.KeepText {
		keep[name] = true
	}
	return &Sanitizer{
		salt: opts.Salt, keep: keep, keepMessages: opts.KeepMessages,
	}, nil
}

// Log returns a sanitized copy of log.
func (s *Sanitizer) Log(log Log) Log {
	out := Log{Start: log.Start}
	out.Entries = make([]Entry, len(log.Entries))
	for i, entry := range log.Entries {
		out.Entries[i] = Entry{
			At: entry.At, Observation: s.Observation(entry.Observation),
		}
	}
	return out
}

// Observation returns a sanitized copy of o.
func (s *Sanitizer) Observation(o inventory.Observation) inventory.Observation {
	out := o
	out.Entity = s.entity(o.Entity)
	out.UID = s.optional("uid", o.UID)
	out.Attributes = s.attributes(o.Attributes)
	out.Targets = nil
	for _, target := range o.Targets {
		out.Targets = append(out.Targets, s.entity(target))
	}
	out.Change = s.change(o.Change)
	out.Note = o.Note
	out.Note.Message = ""
	if s.keepMessages {
		out.Note.Message = o.Note.Message
	}
	out.Note.Reason = s.code(o.Note.Reason)
	// The reporting component can be a custom controller named after
	// the company or product.
	out.Note.Source = s.optional("source", o.Note.Source)
	return out
}

func (s *Sanitizer) entity(id inventory.EntityID) inventory.EntityID {
	if id.Kind == imageKind && id.Name != "" {
		// Image entities share the pseudonym of the image attribute.
		return inventory.EntityID{
			Group: id.Group, Kind: id.Kind,
			Namespace: s.optional("ns", id.Namespace),
			Name:      s.image(id.Name),
		}
	}
	return inventory.EntityID{
		Group:     id.Group,
		Kind:      id.Kind,
		Namespace: s.optional("ns", id.Namespace),
		Name:      s.name(id.Name),
	}
}

// name hides each "/"-separated segment on its own, so a container named
// "pod/app" keeps its pod's pseudonym and matches "containers[app]" paths.
func (s *Sanitizer) name(name string) string {
	if name == "" {
		return ""
	}
	segments := strings.Split(name, "/")
	for i, segment := range segments {
		segments[i] = s.optional("name", segment)
	}
	return strings.Join(segments, "/")
}

func (s *Sanitizer) attributes(
	in map[string]inventory.Value,
) map[string]inventory.Value {
	if in == nil {
		return nil
	}
	out := make(map[string]inventory.Value, len(in))
	for name, value := range in {
		if clean, ok := s.attribute(name, value); ok {
			out[name] = clean
		}
	}
	return out
}

// attribute sanitizes one attribute; false drops it.
func (s *Sanitizer) attribute(
	name string, value inventory.Value,
) (inventory.Value, bool) {
	if _, isNumber := value.AsNumber(); isNumber {
		return value, true
	}
	if _, isBool := value.AsBool(); isBool || isTime(value) ||
		value.IsZero() || s.keep[name] {
		return value, true
	}
	text, ok := s.text(name, value.AsText())
	return inventory.Text(text), ok
}

// text sanitizes a text value by what the attribute or path holds; false
// means the text is free text and is dropped.
func (s *Sanitizer) text(name, text string) (string, bool) {
	switch {
	case text == "":
		return "", true
	case isImage(name):
		return s.image(text), true
	case isLabels(name):
		return s.labels(text), true
	case isFreeText(name) || strings.ContainsAny(text, " \t\n"):
		return "", false
	default:
		return s.code(text), true
	}
}

func (s *Sanitizer) change(c inventory.Change) inventory.Change {
	out := c
	if !c.Entity.IsZero() {
		out.Entity = s.entity(c.Entity)
	}
	out.Actor = s.optional("actor", c.Actor)
	// The GitOps application ("argocd/shop") names teams and products.
	out.App = s.name(c.App)
	out.Revision = s.code(c.Revision)
	out.Fields = nil
	for _, field := range c.Fields {
		path := s.path(field.Path)
		before, _ := s.text(field.Path, field.Before)
		after, _ := s.text(field.Path, field.After)
		if field.Before != field.After && before == after {
			// Dropping free text must not hide that the field changed.
			before = s.pseudonym("text", field.Before)
			after = s.pseudonym("text", field.After)
		}
		out.Fields = append(out.Fields, inventory.FieldChange{
			Path: path, Before: before, After: after,
		})
	}
	return out
}

// imageKind is the entity kind of container images. It is spelled here
// rather than imported so replay stays independent of the Kubernetes
// schema package.
const imageKind inventory.Kind = "image"

// codePattern matches reason codes and plain numbers, which are kept:
// CrashLoopBackOff, Running, OOMKilled, 3, 1.5.
var codePattern = regexp.MustCompile(
	`^([A-Z][A-Za-z0-9]*|[0-9][0-9.]*|true|false)$`)

// code keeps a reason code and hides anything else.
func (s *Sanitizer) code(text string) string {
	if text == "" || codePattern.MatchString(text) {
		return text
	}
	return s.pseudonym("id", text)
}

func (s *Sanitizer) optional(class, text string) string {
	if text == "" {
		return ""
	}
	return s.pseudonym(class, text)
}

// pseudonymBytes is how many hash bytes a pseudonym keeps: ten hex
// digits, short to read and still unlikely to collide within one log.
const pseudonymBytes = 5

// pseudonym derives a stable, short name from the salted hash of text.
func (s *Sanitizer) pseudonym(class, text string) string {
	sum := sha256.Sum256([]byte(s.salt + "\x00" + class + "\x00" + text))
	return class + "-" + hex.EncodeToString(sum[:pseudonymBytes])
}

// image hides the repository and the tag separately, so a rollout still
// reads as the same image with a new tag.
func (s *Sanitizer) image(text string) string {
	repository, digest, hasDigest := strings.Cut(text, "@")
	tag := ""
	slash := strings.LastIndex(repository, "/")
	if colon := strings.LastIndex(repository, ":"); colon > slash {
		repository, tag = repository[:colon], repository[colon+1:]
	}
	out := s.pseudonym("image", repository)
	if tag != "" {
		out += ":" + s.pseudonym("tag", tag)
	}
	if hasDigest {
		out += "@" + s.pseudonym("digest", digest)
	}
	return out
}

// labelValuePattern finds the values of "key=value", "key!=value" and
// "key in (a,b)" terms in label and selector text.
var labelValuePattern = regexp.MustCompile(`(!?=)([^,()]*)|\(([^)]*)\)`)

func (s *Sanitizer) labels(text string) string {
	return labelValuePattern.ReplaceAllStringFunc(text, func(term string) string {
		if strings.HasPrefix(term, "(") {
			values := strings.Split(strings.Trim(term, "()"), ",")
			for i, value := range values {
				values[i] = s.optional("label", strings.TrimSpace(value))
			}
			return "(" + strings.Join(values, ",") + ")"
		}
		operator, value := "=", strings.TrimPrefix(term, "=")
		if strings.HasPrefix(term, "!=") {
			operator, value = "!=", strings.TrimPrefix(term, "!=")
		}
		return operator + s.optional("label", value)
	})
}

// bracketPattern finds named segments in a change path, such as the
// container name in "containers[app].image".
var bracketPattern = regexp.MustCompile(`\[([^\]]*)\]`)

func (s *Sanitizer) path(path string) string {
	return bracketPattern.ReplaceAllStringFunc(path, func(term string) string {
		return "[" + s.name(strings.Trim(term, "[]")) + "]"
	})
}

func isImage(name string) bool {
	return strings.HasSuffix(name, "image")
}

func isLabels(name string) bool {
	return name == "labels" || name == "selector" ||
		strings.HasSuffix(name, ".labels") ||
		strings.HasSuffix(name, ".selector")
}

func isFreeText(name string) bool {
	return strings.HasSuffix(name, "message") ||
		strings.HasSuffix(name, "error") || strings.HasSuffix(name, ".output")
}

// isTime reports whether value is a timestamp, including the zero time.
func isTime(value inventory.Value) bool {
	return value.Equal(inventory.Time(value.AsTime()))
}
