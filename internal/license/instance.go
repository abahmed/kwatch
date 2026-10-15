package license

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

// InstanceKeyStore persists this installation's Ed25519 instance key. The
// app layer implements it (for example with a Kubernetes Secret).
type InstanceKeyStore interface {
	// Load returns the stored key. ok is false when none has been saved yet.
	Load(ctx context.Context) (key ed25519.PrivateKey, ok bool, err error)
	Save(ctx context.Context, key ed25519.PrivateKey) error
}

// EnsureInstanceKey returns the stored instance key, generating and saving a
// new one when none exists (or the stored one is malformed).
func EnsureInstanceKey(
	ctx context.Context, store InstanceKeyStore,
) (ed25519.PrivateKey, error) {
	key, ok, err := store.Load(ctx)
	if err != nil {
		return nil, fmt.Errorf("load instance key: %w", err)
	}
	if ok && len(key) == ed25519.PrivateKeySize {
		return key, nil
	}
	_, key, err = ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate instance key: %w", err)
	}
	if err := store.Save(ctx, key); err != nil {
		return nil, fmt.Errorf("save instance key: %w", err)
	}
	return key, nil
}

// PublicString encodes pub as base64url without padding, the wire form of
// instance_pub.
func PublicString(pub ed25519.PublicKey) string {
	return base64.RawURLEncoding.EncodeToString(pub)
}
