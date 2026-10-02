package compose

import "testing"

// The first letter is changed whole, even when it takes several bytes.
func TestFirstLetterCaseIsRuneSafe(t *testing.T) {
	upper := map[string]string{
		"": "", "a": "A", "élan vital": "Élan vital", "über": "Über",
		"日本": "日本",
	}
	for in, want := range upper {
		if got := upperFirst(in); got != want {
			t.Errorf("upperFirst(%q) = %q, want %q", in, got, want)
		}
	}
	lower := map[string]string{
		"": "", "A": "a", "Élan vital": "élan vital", "Über": "über",
		"TLS expired": "TLS expired",
	}
	for in, want := range lower {
		if got := lowerFirst(in); got != want {
			t.Errorf("lowerFirst(%q) = %q, want %q", in, got, want)
		}
	}
}
