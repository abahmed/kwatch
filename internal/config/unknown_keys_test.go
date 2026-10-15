package config

import (
	"os"
	"strings"
	"testing"
)

func loadConfigText(t *testing.T, text string) *Config {
	t.Helper()
	path := t.TempDir() + "/config.yaml"
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CONFIG_FILE", path)
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	return cfg
}

func TestLoadConfigWarnsOncePerUnknownKey(t *testing.T) {
	cfg := loadConfigText(t, "maxRecentLogLines: 5\n"+
		"app:\n  clusterName: dev\n  clusterNmae: typo\n"+
		"namespaces: [shop]\n")

	if cfg.App.ClusterName != "dev" {
		t.Fatalf("known keys must still load: %q", cfg.App.ClusterName)
	}
	var unknown []string
	for _, warning := range Warnings(cfg) {
		if strings.Contains(warning, "not recognized") {
			unknown = append(unknown, warning)
		}
	}
	if len(unknown) != 2 ||
		!strings.Contains(unknown[0], "(line 1: maxRecentLogLines)") ||
		!strings.Contains(unknown[1], "(line 4: app.clusterNmae)") {
		t.Fatalf("unknown key warnings = %v", unknown)
	}
}

func TestLoadConfigKnownKeysProduceNoUnknownWarning(t *testing.T) {
	cfg := loadConfigText(t, "app:\n  clusterName: dev\n")
	for _, warning := range Warnings(cfg) {
		if strings.Contains(warning, "not recognized") {
			t.Fatalf("unexpected warning: %s", warning)
		}
	}
}

func TestUnknownConfigKeysReportFileLinesAndFullPaths(t *testing.T) {
	t.Setenv("KWATCH_TEST_CLUSTER", "dev")
	text := "# kwatch configuration\n" +
		"\n" +
		"app:\n" +
		"  clusterName: ${KWATCH_TEST_CLUSTER}\n" +
		"\n" +
		"  # a typo follows\n" +
		"  clusterNmae: typo\n" +
		"silences:\n" +
		"  - namespaces: [shop]\n" +
		"  - reasons: [OOMKilled]\n" +
		"    bogus: true\n" +
		"retired: 1\n"
	cfg := loadConfigText(t, text)

	want := []string{
		"line 7: app.clusterNmae",
		"line 11: silences[1].bogus",
		"line 12: retired",
	}
	if strings.Join(cfg.unknownKeys, "|") != strings.Join(want, "|") {
		t.Fatalf("unknown keys = %q, want %q", cfg.unknownKeys, want)
	}
}

func TestUnknownConfigKeysIgnoreTypeErrorsOfReferences(t *testing.T) {
	raw := []byte("resyncSeconds: ${RESYNC}\nbogus: 1\n")
	got := unknownConfigKeys(raw)
	if len(got) != 1 || got[0] != "line 2: bogus" {
		t.Fatalf("unknown keys = %q", got)
	}
}

func TestLintStrictReportsUnknownKeyPath(t *testing.T) {
	path := t.TempDir() + "/config.yaml"
	text := "app:\n  clusterName: dev\n  clusterNmae: typo\n"
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CONFIG_FILE", path)

	err := LintStrict()
	if err == nil || !strings.Contains(err.Error(), "line 3: app.clusterNmae") {
		t.Fatalf("LintStrict error = %v", err)
	}
}

func TestLintStrictAcceptsKnownKeys(t *testing.T) {
	path := t.TempDir() + "/config.yaml"
	if err := os.WriteFile(path, []byte("app:\n  clusterName: dev\n"),
		0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CONFIG_FILE", path)

	if err := LintStrict(); err != nil {
		t.Fatalf("LintStrict error = %v", err)
	}
}

func TestLicenseKeyComesFromTheFileWithEnvExpansion(t *testing.T) {
	t.Setenv("MY_LICENSE", "KW1-from-expansion")
	cfg := loadConfigText(t, "license:\n  key: ${MY_LICENSE}\n")

	if cfg.License.Key != "KW1-from-expansion" {
		t.Fatalf("key = %q", cfg.License.Key)
	}
	if cfg.License.Endpoint != DefaultLicenseEndpoint {
		t.Fatalf("endpoint = %q", cfg.License.Endpoint)
	}
	if got := cfg.Runtime.Lifecycle().License(); got != cfg.License {
		t.Fatalf("runtime license = %+v", got)
	}
	if keys := cfg.unknownKeys; len(keys) != 0 {
		t.Fatalf("license keys reported unknown: %v", keys)
	}
}

func TestLicenseKeyEnvironmentWinsOverTheFile(t *testing.T) {
	t.Setenv("KWATCH_LICENSE_KEY", "  KW1-from-env \n")
	cfg := loadConfigText(t, "license:\n  key: KW1-from-file\n"+
		"  endpoint: http://127.0.0.1:9\n")

	if cfg.License.Key != "KW1-from-env" {
		t.Fatalf("key = %q, want the environment value", cfg.License.Key)
	}
	if cfg.License.Endpoint != "http://127.0.0.1:9" {
		t.Fatalf("endpoint = %q", cfg.License.Endpoint)
	}
}

func TestLicenseKeyEnvironmentAloneEnablesTheKey(t *testing.T) {
	t.Setenv("KWATCH_LICENSE_KEY", "KW1-only-env")
	cfg := loadConfigText(t, "")
	if cfg.License.Key != "KW1-only-env" {
		t.Fatalf("key = %q", cfg.License.Key)
	}
}

func TestLicenseSettingsAreValidated(t *testing.T) {
	for name, license := range map[string]License{
		"space in key":     {Key: "KW1-a b"},
		"newline in key":   {Key: "KW1-a\nb"},
		"relative url":     {Endpoint: "api.kwatch.dev"},
		"unsupported kind": {Endpoint: "ftp://api.kwatch.dev"},
	} {
		errs := validateLicense(license)
		if len(errs) != 1 {
			t.Fatalf("%s: errors = %v", name, errs)
		}
		if strings.Contains(errs[0].Error(), "KW1-") {
			t.Fatalf("%s: the key leaked into %q", name, errs[0])
		}
	}
	if errs := validateLicense(License{
		Key: "KW1-fine", Endpoint: DefaultLicenseEndpoint,
	}); len(errs) != 0 {
		t.Fatalf("valid settings rejected: %v", errs)
	}
}
