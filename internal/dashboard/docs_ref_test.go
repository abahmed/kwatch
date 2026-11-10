package dashboard

import (
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/version"
)

func TestDocsRefPicksTheTagOrMain(t *testing.T) {
	for v, want := range map[string]string{
		"v1.0.0-rc.7":        "v1.0.0-rc.7",
		"1.0.0":              "v1.0.0",
		"v1.2.3":             "v1.2.3",
		"dev":                "main",
		"":                   "main",
		"v1.0.0-3-gabc1234":  "main",
		"v1.0.0-rc.7-dirty":  "main",
		"v1.0.0-rc.7-2-gabc": "main",
		"abc1234":            "main",
		"ci":                 "main",
	} {
		if got := docsRef(v); got != want {
			t.Errorf("docsRef(%q) = %q, want %q", v, got, want)
		}
	}
}

func TestShellLinksTheDocsOfTheRunningVersion(t *testing.T) {
	shell := string(mustAssets().shell)
	want := "https://github.com/abahmed/kwatch/blob/" +
		docsRef(version.Short()) + "/docs/dashboard.md#how-to-get-the-password"
	if !strings.Contains(shell, want) {
		t.Errorf("sign-in link is not %s", want)
	}
	got := string(withDocsRef([]byte("a/" + docsRefMark + "/b")))
	if strings.Contains(got, "{{") {
		t.Errorf("mark left in %q", got)
	}
}
