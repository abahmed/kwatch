// Package license verifies kwatch Pro license keys and activation leases
// offline.
//
// A license key (KW1-...) and a lease (KWL1-...) are both Ed25519-signed JSON
// payloads encoded as base64url(payload) "." base64url(signature), without
// padding. Verification only needs the trusted public keys compiled into the
// binary; this package performs no network I/O.
//
// Evaluate is a pure function that combines a key, a lease, the cluster
// binding, a monotonic clock guard and an optional server verdict into a
// single State that callers use to gate Pro features.
package license
