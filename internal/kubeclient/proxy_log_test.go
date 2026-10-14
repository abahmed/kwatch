package kubeclient

import (
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/config"
)

// An invalid proxy must stop startup, and the error must not quote the
// credentials in the URL.
func TestNewHTTPClientInvalidProxyFailsWithoutCredentials(t *testing.T) {
	client, err := NewHTTPClient(config.ApplicationRuntime{
		ProxyURL: "http://user:s3cret@proxy:bad-port/",
	})

	if err == nil || client != nil {
		t.Fatalf("want an error and no client, got %v, %v", client, err)
	}
	if strings.Contains(err.Error(), "s3cret") ||
		strings.Contains(err.Error(), "user:") {
		t.Fatalf("proxy credentials in the error: %v", err)
	}
}
