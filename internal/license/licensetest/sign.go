package licensetest

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"

	"github.com/abahmed/kwatch/internal/license"
)

func sign(prefix string, payload []byte, priv ed25519.PrivateKey) string {
	sig := ed25519.Sign(priv, payload)
	return prefix +
		base64.RawURLEncoding.EncodeToString(payload) + "." +
		base64.RawURLEncoding.EncodeToString(sig)
}

// SignKey encodes and signs a license key.
func SignKey(k license.Key, priv ed25519.PrivateKey) string {
	payload, _ := json.Marshal(k)
	return sign("KW1-", payload, priv)
}

// SignLease encodes and signs a lease.
func SignLease(l license.Lease, priv ed25519.PrivateKey) string {
	payload, _ := json.Marshal(l)
	return sign("KWL1-", payload, priv)
}
