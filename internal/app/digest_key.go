package app

import (
	"crypto/rand"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/storage"
)

// stateDigestKey holds the key for Secret and ConfigMap value digests.
// It lives in the state file, never in a cluster Secret.
const stateDigestKey = "inventory.digest-key"

const digestKeySize = 32

// EnsureDigestKey returns the stored digest key, creating and storing a
// random one on first use. The same key across restarts keeps Secret and
// ConfigMap digests comparable, so changes made while kwatch was down are
// found and unchanged objects are not reported. A store reset makes a new
// key, which is fine: a reset forgets the old digests anyway.
func (d diskState) EnsureDigestKey() ([]byte, error) {
	var key []byte
	found, err := d.get(stateDigestKey, &key)
	if err != nil {
		return nil, err
	}
	if found && len(key) == digestKeySize {
		return key, nil
	}
	key = make([]byte, digestKeySize)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if err := d.put(stateDigestKey, key); err != nil {
		return nil, err
	}
	return key, nil
}

// sourceDigestKey loads the digest key for this session. Without one the
// source falls back to a per-process key: monitoring still works, only
// changes made while kwatch was down are reported for every Secret and
// ConfigMap.
func sourceDigestKey(state *storage.Store) []byte {
	key, err := diskState{store: state}.EnsureDigestKey()
	if err != nil {
		klog.ErrorS(err, "load digest key; using a per-process key",
			"component", "pipeline", "operation", "digest_key")
		return nil
	}
	return key
}
