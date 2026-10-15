package main

import (
	"bytes"
	"strings"
	"testing"
)

func runLicenseCmd(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := runCommand(append([]string{"license"}, args...), &out, &errOut,
		func() int { t.Fatal("license must not start the server"); return 0 })
	return code, out.String(), errOut.String()
}

func TestLicenseStatusWithoutAKeyIsCommunity(t *testing.T) {
	t.Setenv("CONFIG_FILE", "")
	t.Setenv("KWATCH_LICENSE_KEY", "")
	code, out, errOut := runLicenseCmd(t, "status")
	if code != 0 || !strings.Contains(out, "Community") {
		t.Fatalf("code=%d out=%q err=%q", code, out, errOut)
	}
}

func TestLicenseStatusMasksTheKeyAndUsesNoNetwork(t *testing.T) {
	const key = "KW1-eyJ2IjoxfQ.c2ln-abcdWXYZ"
	t.Setenv("CONFIG_FILE", "")
	t.Setenv("KWATCH_LICENSE_KEY", key)
	code, out, errOut := runLicenseCmd(t, "status")
	if code != 0 {
		t.Fatalf("code=%d err=%q", code, errOut)
	}
	if strings.Contains(out+errOut, key) {
		t.Fatalf("the key was printed: %q", out)
	}
	if !strings.Contains(out, "KW1-…WXYZ") ||
		!strings.Contains(out, "invalid") {
		t.Fatalf("out=%q", out)
	}
}

func TestLicenseDeactivateFailsOutsideACluster(t *testing.T) {
	t.Setenv("CONFIG_FILE", "")
	t.Setenv("KWATCH_LICENSE_KEY", "KW1-eyJ2IjoxfQ.c2ln-abcdWXYZ")
	code, _, _ := runLicenseCmd(t, "deactivate")
	if code == 0 {
		t.Fatal("a key that cannot be released must exit non-zero")
	}
}

func TestLicenseDeactivateWithoutAKeyHasNothingToDo(t *testing.T) {
	t.Setenv("CONFIG_FILE", "")
	t.Setenv("KWATCH_LICENSE_KEY", "")
	code, out, _ := runLicenseCmd(t, "deactivate")
	if code != 0 || !strings.Contains(out, "nothing to deactivate") {
		t.Fatalf("code=%d out=%q", code, out)
	}
}

func TestLicenseRejectsUnknownUsage(t *testing.T) {
	for _, args := range [][]string{{}, {"nope"}, {"status", "extra"}} {
		code, _, errOut := runLicenseCmd(t, args...)
		if code != 2 || !strings.Contains(errOut, "usage: license") {
			t.Fatalf("%v: code=%d err=%q", args, code, errOut)
		}
	}
}
