package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
)

// TestClassifyPullIgnoresStatusLikeTags: a tag such as "1.503" or
// "429" is not an HTTP status; only a code the registry answered with
// classifies the pull.
func TestClassifyPullIgnoresStatusLikeTags(t *testing.T) {
	cases := map[string]detection.Mode{
		`Failed to pull image "registry.example.com/app:1.503": rpc ` +
			`error: code = Unknown desc = unexpected EOF`: pullImage,
		`Back-off pulling image "r.example/app:429"`:    pullImage,
		`Failed to pull image "r.example/app:v401-403"`: pullImage,
		`Failed to pull image "registry.example.com/app:1.503": ` +
			`unexpected status code 503 Service Unavailable`: pullServer,
		`pulling "app:2": received unexpected HTTP status: ` +
			`500 Internal Server Error`: pullServer,
		`failed to fetch anonymous token: ` +
			`unexpected status: 401`: pullAuth,
		`GET https://r.example/v2/: HTTP/1.1 429`: pullRateLimit,
		`failed to authorize: 403`:                pullAuth,
	}
	for text, want := range cases {
		if got := classifyPull(text); got != want {
			t.Errorf("classifyPull(%q) = %q, want %q", text, got, want)
		}
	}
}
