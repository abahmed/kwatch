package license

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Token prefixes.
const (
	keyPrefix   = "KW1-"
	leasePrefix = "KWL1-"
)

// Plans.
const (
	PlanTrial = "trial"
	PlanPro   = "pro"
)

// Errors returned by verification.
var (
	ErrMalformed    = errors.New("license: malformed")
	ErrBadSignature = errors.New("license: bad signature")
	ErrUnknownKID   = errors.New("license: unknown key id")
	ErrExpired      = errors.New("license: expired")
)

// Key is the signed payload of a license key.
type Key struct {
	V    int    `json:"v"`
	KID  string `json:"kid"`
	LID  string `json:"lid"`
	Acc  string `json:"acc"`
	Plan string `json:"plan"`
	Iat  int64  `json:"iat"`
	Exp  int64  `json:"exp"`
	Max  int    `json:"max"`
}

// ExpiresAt returns the license expiry time.
func (k Key) ExpiresAt() time.Time { return time.Unix(k.Exp, 0).UTC() }

func malformed(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrMalformed, fmt.Sprintf(format, args...))
}

// split decodes "<prefix><payload>.<sig>" into raw payload and signature.
func split(prefix, s string) (payload, sig []byte, err error) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, prefix) {
		return nil, nil, malformed("missing %q prefix", prefix)
	}
	parts := strings.Split(strings.TrimPrefix(s, prefix), ".")
	if len(parts) != 2 {
		return nil, nil, malformed("expected payload and signature")
	}
	payload, err = base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil || len(payload) == 0 {
		return nil, nil, malformed("bad payload encoding")
	}
	sig, err = base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || len(sig) != ed25519.SignatureSize {
		return nil, nil, malformed("bad signature encoding")
	}
	return payload, sig, nil
}

func sign(prefix string, payload []byte, priv ed25519.PrivateKey) string {
	sig := ed25519.Sign(priv, payload)
	return prefix +
		base64.RawURLEncoding.EncodeToString(payload) + "." +
		base64.RawURLEncoding.EncodeToString(sig)
}

// SignKey encodes and signs a license key.
func SignKey(k Key, priv ed25519.PrivateKey) string {
	payload, _ := json.Marshal(k)
	return sign(keyPrefix, payload, priv)
}

func decodeKey(payload []byte) (Key, error) {
	var k Key
	if err := json.Unmarshal(payload, &k); err != nil {
		return Key{}, malformed("bad payload json")
	}
	if k.V != 1 {
		return Key{}, malformed("unsupported version %d", k.V)
	}
	if k.KID == "" || k.LID == "" || k.Exp <= 0 {
		return Key{}, malformed("missing required fields")
	}
	if k.Plan != PlanTrial && k.Plan != PlanPro {
		return Key{}, malformed("unknown plan %q", k.Plan)
	}
	return k, nil
}

// ParseKey decodes a license key without verifying its signature.
func ParseKey(s string) (Key, error) {
	payload, _, err := split(keyPrefix, s)
	if err != nil {
		return Key{}, err
	}
	return decodeKey(payload)
}

func (v *Verifier) check(kid string, payload, sig []byte) error {
	pub, ok := v.keys[kid]
	if !ok || len(pub) != ed25519.PublicKeySize {
		return fmt.Errorf("%w: %q", ErrUnknownKID, kid)
	}
	if !ed25519.Verify(pub, payload, sig) {
		return ErrBadSignature
	}
	return nil
}

// VerifyKey parses s, verifies its signature and checks expiry at now.
// On ErrExpired the parsed key is still returned.
func (v *Verifier) VerifyKey(s string, now time.Time) (Key, error) {
	payload, sig, err := split(keyPrefix, s)
	if err != nil {
		return Key{}, err
	}
	k, err := decodeKey(payload)
	if err != nil {
		return Key{}, err
	}
	if err := v.check(k.KID, payload, sig); err != nil {
		return Key{}, err
	}
	if !now.Before(k.ExpiresAt()) {
		return k, ErrExpired
	}
	return k, nil
}
