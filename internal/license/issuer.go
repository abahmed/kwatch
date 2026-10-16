package license

import "crypto/ed25519"

// SignLease encodes and signs a lease. The production issuer is the license
// Worker; this exists for tests and tooling.
func SignLease(l Lease, priv ed25519.PrivateKey) string {
	return signLease(l, priv)
}
