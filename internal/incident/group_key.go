package incident

import (
	"fmt"
	"strings"
)

// GroupKeyCodec owns the serialized smart-group key format. The format is
// intentionally stable because keys can be present in persisted group state.
type GroupKeyCodec struct{}

// GroupKeyKind identifies the stable scope encoded by a smart-group key.
// These values are an internal vocabulary; the serialized key remains the
// existing pipe-delimited format for persistence compatibility.
type GroupKeyKind string

const (
	GroupKeyOwner     GroupKeyKind = "owner"
	GroupKeySignature GroupKeyKind = "sig"
	GroupKeyGlobal    GroupKeyKind = "global"
	GroupKeyNode      GroupKeyKind = "node"
	GroupKeyService   GroupKeyKind = "svc"
	GroupKeyControl   GroupKeyKind = "cp"
	GroupKeyImage     GroupKeyKind = "img"
)

// EncodeOwner returns the key shared by incidents for one workload owner.
func (GroupKeyCodec) EncodeOwner(reason, namespace, owner string) string {
	return reason + "|" + namespace + "|" + owner
}

// EncodeScope returns a key shared by incidents in a named scope.
func (GroupKeyCodec) EncodeScope(reason, kind, value string) string {
	return reason + "|" + kind + "|" + value
}

// EncodeImage returns the key shared by image-related incidents in a
// namespace.
func (GroupKeyCodec) EncodeImage(reason, image, namespace string) string {
	return reason + "|img|" + image + "|ns|" + namespace
}

// EncodeFanOut returns the internal namespace fan-out key. It uses the same
// owner-shaped wire form with a reserved wildcard owner and is not exposed as
// a persisted incident identity.
func (GroupKeyCodec) EncodeFanOut(reason, namespace string) string {
	return reason + "|" + namespace + "|*"
}

// ParsedGroupKey is the structured form of a persisted smart-group key.
// OwnerForm distinguishes the ordinary reason/namespace/owner shape from the
// other three-part forms.
type ParsedGroupKey struct {
	Reason    string
	Namespace string
	Owner     string
	Kind      string
	Value     string
	OwnerForm bool
}

// Parse accepts all key shapes currently written by Kwatch. A three-part key
// whose middle component is "ns" remains an owner key: that is the only
// interpretation that is safe for an owner literally named "ns". Namespace
// scope keys are still written and read by their existing group path.
func (GroupKeyCodec) Parse(key string) (ParsedGroupKey, bool) {
	parts := strings.Split(key, "|")
	if len(parts) == 3 {
		switch parts[1] {
		case "sig", "global", "node", "svc", "cp":
			return ParsedGroupKey{
				Reason: parts[0], Kind: parts[1], Value: parts[2],
				Namespace: groupNamespace(parts[1], parts[2]),
			}, true
		default:
			return ParsedGroupKey{
				Reason: parts[0], Namespace: parts[1], Owner: parts[2],
				OwnerForm: true,
			}, true
		}
	}
	if len(parts) == 5 && parts[1] == "img" && parts[3] == "ns" {
		return ParsedGroupKey{
			Reason: parts[0], Kind: parts[1], Value: parts[2],
			Namespace: parts[4],
		}, true
	}
	return ParsedGroupKey{}, false
}

// Kind returns the semantic scope encoded by key without exposing parsing
// details to callers that only need to branch on the group type.
func (c GroupKeyCodec) Kind(key string) (GroupKeyKind, bool) {
	parsed, ok := c.Parse(key)
	if !ok {
		return "", false
	}
	if parsed.OwnerForm {
		return GroupKeyOwner, true
	}
	return GroupKeyKind(parsed.Kind), true
}

// Validate checks that key is one of the supported persisted group-key forms.
// It intentionally does not reject empty components: older configurations can
// produce those values, and Parse must remain backwards-compatible.
func (c GroupKeyCodec) Validate(key string) error {
	if _, ok := c.Parse(key); !ok {
		return fmt.Errorf("invalid group key %q", key)
	}
	return nil
}

var groupKeyCodec GroupKeyCodec
