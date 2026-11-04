package webpush

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/url"
	"time"
)

// Subject is the contact push services see in every VAPID signature
// (RFC 8292): the project, never an address of the cluster.
const Subject = "https://github.com/abahmed/kwatch"

// tokenLife is how long a signature is valid; push services refuse
// more than 24 hours.
const tokenLife = 12 * time.Hour

// Key is kwatch's VAPID key pair. A browser subscribes with its public
// half, and its push service then takes only messages signed with the
// private half, so the key must outlive restarts.
type Key struct {
	private *ecdsa.PrivateKey
}

// NewKey makes a new P-256 key pair.
func NewKey() (Key, error) {
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return Key{}, fmt.Errorf("webpush: new key: %w", err)
	}
	return Key{private: k}, nil
}

// ParseKey reads a key Marshal wrote.
func ParseKey(s string) (Key, error) {
	raw, err := decode(s)
	if err != nil {
		return Key{}, fmt.Errorf("webpush: stored key: %w", err)
	}
	k, err := ecdsa.ParseRawPrivateKey(elliptic.P256(), raw)
	if err != nil {
		return Key{}, fmt.Errorf("webpush: stored key: %w", err)
	}
	return Key{private: k}, nil
}

// Marshal writes the private key as base64url, for ParseKey.
func (k Key) Marshal() (string, error) {
	raw, err := k.private.Bytes()
	if err != nil {
		return "", fmt.Errorf("webpush: marshal key: %w", err)
	}
	return encode(raw), nil
}

// Public is the public key as the browser's pushManager.subscribe
// takes it (applicationServerKey): base64url, uncompressed point.
func (k Key) Public() string {
	raw, err := k.private.PublicKey.Bytes()
	if err != nil {
		return ""
	}
	return encode(raw)
}

// authorization is the Authorization header of a message to endpoint:
// a JWT for the push service's origin, signed ES256 (RFC 8292).
func (k Key) authorization(endpoint *url.URL, now time.Time) (string, error) {
	claims, err := json.Marshal(struct {
		Aud string `json:"aud"`
		Exp int64  `json:"exp"`
		Sub string `json:"sub"`
	}{endpoint.Scheme + "://" + endpoint.Host,
		now.Add(tokenLife).Unix(), Subject})
	if err != nil {
		return "", err
	}
	signed := encode([]byte(`{"typ":"JWT","alg":"ES256"}`)) + "." +
		encode(claims)
	digest := sha256.Sum256([]byte(signed))
	r, s, err := ecdsa.Sign(rand.Reader, k.private, digest[:])
	if err != nil {
		return "", fmt.Errorf("webpush: sign: %w", err)
	}
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	return "vapid t=" + signed + "." + encode(sig) + ", k=" + k.Public(),
		nil
}
