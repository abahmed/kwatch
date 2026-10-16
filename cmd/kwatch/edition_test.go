//go:build !licensetest

package main

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestVersionJSONNamesTheEdition(t *testing.T) {
	var out, errOut bytes.Buffer
	code := runCommand([]string{"version", "--json"}, &out, &errOut,
		func() int { return 99 })
	if code != 0 {
		t.Fatalf("code=%d err=%q", code, errOut.String())
	}
	var got struct {
		Version string `json:"version"`
		Edition string `json:"edition"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Edition != "community" || got.Version == "" {
		t.Fatalf("version json = %s", out.String())
	}
}

func TestReplayExportIsAProFeatureInCommunity(t *testing.T) {
	var out, errOut bytes.Buffer
	called := false
	code := runCommand([]string{"replay-export", "--since", "2h", "--out",
		t.TempDir() + "/o.jsonl"}, &out, &errOut,
		func() int { called = true; return 0 })
	if code == 0 || called {
		t.Fatalf("code=%d called=%v", code, called)
	}
	if got := strings.TrimSpace(errOut.String()); got !=
		"replay-export is a kwatch Pro feature" {
		t.Fatalf("stderr = %q", got)
	}
	if out.Len() != 0 {
		t.Fatalf("stdout = %q", out.String())
	}
}

func TestLintReportsProConfigurationAsInformation(t *testing.T) {
	path := t.TempDir() + "/config.yaml"
	body := "runbooks:\n  OOMKilled: https://wiki/oom\n" +
		"severityByReason:\n  OOMKilled: critical\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CONFIG_FILE", path)
	var out, errOut bytes.Buffer

	code := runLint(false, false, &out, &errOut)

	if code != 0 {
		t.Fatalf("lint must not fail on Pro config: %d %s", code,
			errOut.String())
	}
	for _, want := range []string{
		"info: policy.runbooks is a kwatch Pro feature",
		"info: policy.severity is a kwatch Pro feature", "config OK",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("missing %q in %q", want, out.String())
		}
	}
}

func TestLintSaysNothingAboutProForAPlainConfig(t *testing.T) {
	path := t.TempDir() + "/config.yaml"
	if err := os.WriteFile(path, []byte("app:\n  clusterName: dev\n"),
		0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CONFIG_FILE", path)
	var out, errOut bytes.Buffer

	if code := runLint(false, false, &out, &errOut); code != 0 {
		t.Fatalf("code = %d", code)
	}
	if strings.Contains(out.String(), "is a kwatch Pro feature") {
		t.Fatalf("stdout = %q", out.String())
	}
}

func TestLintNamesTheProDetectorsCommunityDoesNotRun(t *testing.T) {
	path := t.TempDir() + "/config.yaml"
	if err := os.WriteFile(path, []byte("app:\n  clusterName: dev\n"),
		0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CONFIG_FILE", path)
	var out, errOut bytes.Buffer

	if code := runLint(false, false, &out, &errOut); code != 0 {
		t.Fatalf("code = %d", code)
	}
	if !strings.Contains(out.String(),
		"info: Pro detectors not running: ") {
		t.Fatalf("stdout = %q", out.String())
	}
}
