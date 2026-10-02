package matrix

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
)

// newNonce returns a random per-instance token that keeps plain-message
// transaction IDs unique across restarts.
func newNonce() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// transactionID derives the Matrix idempotency token from the logical
// message: room, incident key and action, and rendered content. A retry of
// the same send, including one after a timed-out request that succeeded,
// reuses the token so the homeserver returns the original event instead of
// posting twice. A changed revision renders different content and therefore
// gets a new token. Incident messages are deterministic, so every retry of
// one is deduplicated. Plain messages fold a per-send nonce and counter into
// the identity, so identical texts are never deduplicated by the homeserver;
// a delivery-level retry calls SendMessage again and draws a new counter, so
// a plain notice may post twice after a timed-out send the server applied.
func transactionID(room, identity, plain, formatted string) string {
	h := sha256.New()
	for _, part := range []string{room, identity, plain, formatted} {
		_, _ = h.Write([]byte(part))
		_, _ = h.Write([]byte{0})
	}
	return "kwatch-" + hex.EncodeToString(h.Sum(nil))[:32]
}
