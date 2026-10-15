package license

import "crypto/ed25519"

// trustedKeys maps a key id (kid) to the public key that verifies licenses
// and leases signed under it. Tests inject their own keys through NewVerifier.
var trustedKeys = map[string]ed25519.PublicKey{
	// TODO: real production key is generated in Phase 0.
	// "v1": ed25519.PublicKey{ /* 32 bytes */ },
}

// Verifier verifies keys and leases against a set of trusted public keys.
type Verifier struct {
	keys map[string]ed25519.PublicKey
}

// NewVerifier returns a Verifier trusting a copy of keys.
func NewVerifier(keys map[string]ed25519.PublicKey) *Verifier {
	copied := make(map[string]ed25519.PublicKey, len(keys))
	for kid, key := range keys {
		copied[kid] = key
	}
	return &Verifier{keys: copied}
}

// DefaultVerifier returns a Verifier using the keys compiled into kwatch.
func DefaultVerifier() *Verifier {
	return NewVerifier(trustedKeys)
}
