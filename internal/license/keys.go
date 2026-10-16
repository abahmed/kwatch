package license

import "crypto/ed25519"

// trustedKeys maps a key id (kid) to the public key that verifies licenses
// and leases signed under it. Tests inject their own keys through NewVerifier.
var trustedKeys = map[string]ed25519.PublicKey{
	// Production signing key of the license service at api.kwatch.dev.
	"v1": ed25519.PublicKey{
		0x44, 0x0c, 0x68, 0x1b, 0x7a, 0xc4, 0x07, 0xd9,
		0xdb, 0xc9, 0x6c, 0x3e, 0xda, 0x90, 0x46, 0xe4,
		0xc2, 0xa1, 0x42, 0xfb, 0x14, 0x09, 0x0a, 0xbf,
		0xa4, 0x92, 0xc5, 0x1a, 0x3a, 0xf7, 0xcf, 0x3e,
	},
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
