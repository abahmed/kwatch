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
