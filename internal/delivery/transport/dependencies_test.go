package transport

import (
	"crypto/tls"
	"net/http"
	"testing"
)

func TestDependenciesTLSConfigCopiesClientSettings(t *testing.T) {
	base := &tls.Config{InsecureSkipVerify: true} // #nosec G402
	deps := Dependencies{HTTPClient: &http.Client{
		Transport: &http.Transport{TLSClientConfig: base},
	}}

	got := deps.TLSConfig()

	if got == nil || !got.InsecureSkipVerify {
		t.Fatal("TLS settings were not carried over")
	}
	if got == base {
		t.Fatal("the shared config must be copied")
	}
}

func TestDependenciesTLSConfigIsNilWithoutSettings(t *testing.T) {
	if (Dependencies{}).TLSConfig() != nil {
		t.Fatal("no client means no TLS config")
	}
	plain := Dependencies{HTTPClient: &http.Client{}}
	if plain.TLSConfig() != nil {
		t.Fatal("a default transport carries no TLS config")
	}
}
