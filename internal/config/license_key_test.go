package config

import (
	"strings"
	"testing"
)

func TestLoadConfigStripsWhitespaceFromTheLicenseKey(t *testing.T) {
	cfg := loadConfigText(t,
		"license:\n  key: \"KW1-abc \\n def\\t.ghi\\r\\n\"\n")
	if cfg.License.Key != "KW1-abcdef.ghi" {
		t.Fatalf("key = %q", cfg.License.Key)
	}
	if got := cfg.Runtime.Lifecycle().License().Key; got != cfg.License.Key {
		t.Fatalf("runtime key = %q", got)
	}
}

func TestLoadConfigStripsWhitespaceFromTheEnvironmentKey(t *testing.T) {
	t.Setenv("KWATCH_LICENSE_KEY", "KW1-abc\n def\n")
	cfg := loadConfigText(t, "{}\n")
	if cfg.License.Key != "KW1-abcdef" {
		t.Fatalf("key = %q", cfg.License.Key)
	}
}

func TestLiteralLicenseKeyInTheFileIsWarnedAbout(t *testing.T) {
	cfg := loadConfigText(t, "license:\n  key: KW1-literal\n")
	if !hasWarning(Warnings(cfg), "license.key is written as text") {
		t.Fatalf("no literal key warning: %v", Warnings(cfg))
	}
	if strings.Contains(strings.Join(Warnings(cfg), " "), "KW1-literal") {
		t.Fatal("the warning must not echo the key")
	}
}

func TestExpandedLicenseKeyIsNotWarnedAbout(t *testing.T) {
	t.Setenv("MY_KEY", "KW1-from-env")
	cfg := loadConfigText(t, "license:\n  key: ${MY_KEY}\n")
	if hasWarning(Warnings(cfg), "license.key") {
		t.Fatalf("unexpected warning: %v", Warnings(cfg))
	}
}

func hasWarning(warnings []string, text string) bool {
	for _, w := range warnings {
		if strings.Contains(w, text) {
			return true
		}
	}
	return false
}
