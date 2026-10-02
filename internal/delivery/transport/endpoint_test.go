package transport

import "testing"

func TestValidEndpointAcceptsOnlyAbsoluteHTTPURLs(t *testing.T) {
	cases := map[string]bool{
		"https://hooks.slack.test/services/x": true,
		"http://receiver.local:8080/path":     true,
		" https://padded.test/ ":              true,
		"":                                    false,
		"hooks.slack.test/services/x":         false,
		"ftp://files.test/x":                  false,
		"https://":                            false,
		"https://:443/path":                   false,
		"://broken":                           false,
		"https://bad host.test/":              false,
	}
	for raw, want := range cases {
		if got := ValidEndpoint(raw); got != want {
			t.Errorf("ValidEndpoint(%q) = %v, want %v", raw, got, want)
		}
	}
}
