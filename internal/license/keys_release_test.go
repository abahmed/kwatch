package license

import (
	"crypto/ed25519"
	"os"
	"testing"
)

// TestReleaseTrustedKeys fails a release build that would ship without the
// production signing key. It runs only when RELEASE=1 (the release and
// publish workflows, or `make release-keys-check`), so local verify passes
// while the key is still a placeholder.
func TestReleaseTrustedKeys(t *testing.T) {
	if os.Getenv("RELEASE") != "1" {
		t.Skip("set RELEASE=1 to check the production trusted keys")
	}
	if len(trustedKeys) == 0 {
		t.Fatal("trustedKeys is empty: a release would verify no " +
			"license; add the production key to internal/license/keys.go")
	}
	for kid, key := range trustedKeys {
		if kid == "" || len(key) != ed25519.PublicKeySize {
			t.Fatalf("trusted key %q is not a %d byte ed25519 key",
				kid, ed25519.PublicKeySize)
		}
	}
}
