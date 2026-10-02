package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestRunLintWarnsWithoutFailingWhenNoProviderIsConfigured(t *testing.T) {
	path := t.TempDir() + "/config.yaml"
	if err := os.WriteFile(path, []byte("app:\n  clusterName: dev\n"),
		0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CONFIG_FILE", path)
	var out, errOut bytes.Buffer

	code := runLint(false, false, &out, &errOut)

	if code != 0 {
		t.Fatalf("exit code = %d, stderr = %s", code, errOut.String())
	}
	if !strings.Contains(out.String(),
		"warning: no alert providers configured") {
		t.Fatalf("stdout = %q", out.String())
	}
}

func lintConfig(t *testing.T, body string) (int, string, string) {
	t.Helper()
	path := t.TempDir() + "/config.yaml"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CONFIG_FILE", path)
	var out, errOut bytes.Buffer
	code := runLint(false, false, &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestRunLintRejectsProviderConstructionFailure(t *testing.T) {
	secret := t.TempDir() + "/token"
	if err := os.WriteFile(secret, []byte("tok"), 0o600); err != nil {
		t.Fatal(err)
	}
	code, _, errOut := lintConfig(t, "alert:\n  matrix:\n"+
		"    homeServer: \"not a url\"\n"+
		"    accessToken: ${file:"+secret+"}\n    internalRoomId: r\n")
	if code != 1 || !strings.Contains(errOut, "could not be constructed") {
		t.Fatalf("code=%d stderr=%q", code, errOut)
	}
}

func TestRunLintRejectsMissingRequiredProviderField(t *testing.T) {
	code, _, errOut := lintConfig(t, "alert:\n  discord:\n    webhook: \"\"\n")
	if code != 1 || !strings.Contains(errOut, "alert.discord.webhook") {
		t.Fatalf("code=%d stderr=%q", code, errOut)
	}
}

func TestRunLintAcceptsShippedConfig(t *testing.T) {
	t.Setenv("CONFIG_FILE", "../../deploy/config.yaml")
	var out, errOut bytes.Buffer
	if code := runLint(false, false, &out, &errOut); code != 0 {
		t.Fatalf("code=%d stderr=%q", code, errOut.String())
	}
}
