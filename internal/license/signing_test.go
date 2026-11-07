package license

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
)

// The shipped binary only verifies; signing lives in tests. The
// production issuer is the license Worker.

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

// SignLease encodes and signs a lease.
func SignLease(l Lease, priv ed25519.PrivateKey) string {
	payload, _ := json.Marshal(l)
	return sign(leasePrefix, payload, priv)
}
