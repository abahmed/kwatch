package upgrader

import "testing"

// The update check used to be a string inequality, which would nag a v0.12.0
// install about a v0.9.5 release and a build running ahead of the latest
// published release about that release.
func TestIsNewer(t *testing.T) {
	cases := []struct {
		latest, current string
		want            bool
	}{
		{"v0.11.0", "v0.10.5", true},
		{"v0.10.5", "v0.11.0", false}, // running ahead of the latest release
		{"v0.11.0", "v0.11.0", false},
		{"v0.9.5", "v0.12.0", false}, // lexically greater, semantically older
		{"v1.0.0", "v0.99.99", true},
		{"0.11.0", "v0.10.5", true}, // prefix optional
		{"v0.11.1", "v0.11.0", true},
		{
			"v0.11.0",
			"v0.11.0-rc.4",
			false,
		}, // pre-release suffix ignored, same core
		{"nightly", "v0.11.0", true}, // unparsable: differ means report
		{"v0.11.0", "v0.11.0", false},
	}
	for _, c := range cases {
		if got := isNewer(c.latest, c.current); got != c.want {
			t.Errorf(
				"isNewer(%q, %q) = %v, want %v",
				c.latest,
				c.current,
				got,
				c.want,
			)
		}
	}
}
