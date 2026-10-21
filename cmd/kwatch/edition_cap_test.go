//go:build !licensetest

package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// secretFile writes value to a file and returns the ${file:} reference the
// configuration must use for a credential.
func secretFile(t *testing.T, value string) string {
	t.Helper()
	path := t.TempDir() + "/secret"
	if err := os.WriteFile(path, []byte(value), 0o600); err != nil {
		t.Fatal(err)
	}
	return "${file:" + path + "}"
}

// twoProviders configures two chat providers. The Community edition keeps
// one of them (discord, first by name) and leaves the other inactive.
func twoProviders(t *testing.T) string {
	t.Helper()
	return "alert:\n  discord:\n    webhook: " + secretFile(t,
		"https://discord.com/api/webhooks/1/token") +
		"\n  slack:\n    webhook: " + secretFile(t,
		"https://hooks.slack.com/services/T/B/x") + "\n"
}

// communityHome points the command at a data directory with no state
// file, so the installation reads as the Community edition.
func communityHome(t *testing.T, config string) {
	t.Helper()
	path := t.TempDir() + "/config.yaml"
	if err := os.WriteFile(path, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CONFIG_FILE", path)
	t.Setenv("KWATCH_DATA_DIR", t.TempDir())
}

// Lint builds the providers the edition runs, so a provider the cap
// leaves inactive is never constructed: its broken settings cannot fail a
// lint that startup would pass.
func TestLintBuildsOnlyTheProviderCommunityKeeps(t *testing.T) {
	communityHome(t, "alert:\n  discord:\n    webhook: "+secretFile(t,
		"https://discord.com/api/webhooks/1/token")+
		"\n  matrix:\n    homeServer: \"not a url\"\n    accessToken: "+
		secretFile(t, "tok")+"\n    internalRoomId: r\n")
	var out, errOut bytes.Buffer

	code := runLint(false, false, testNow, &out, &errOut)

	if code != 0 {
		t.Fatalf("the inactive provider was built: code=%d stderr=%q",
			code, errOut.String())
	}
	want := "info: Community sends to: discord (inactive: matrix)"
	if !strings.Contains(out.String(), want) {
		t.Fatalf("missing %q in %q", want, out.String())
	}
}

// The picker lists the providers a test would reach: the Community
// edition's one, with a note naming the ones left inactive.
func TestTestAlertReachesOnlyTheProviderCommunityKeeps(t *testing.T) {
	communityHome(t, twoProviders(t))
	var out, errOut bytes.Buffer

	code := runTestAlert([]string{"--provider", "?"}, &out, &errOut,
		testNow)

	if code != 0 {
		t.Fatalf("code=%d stderr=%q", code, errOut.String())
	}
	got := out.String()
	note := "The Community edition sends to discord (inactive: slack)"
	if !strings.Contains(got, note) {
		t.Fatalf("missing %q in %q", note, got)
	}
	names := got[strings.LastIndex(got, "\n\n")+2:]
	if names != "discord\n" {
		t.Fatalf("providers offered = %q, want only discord", names)
	}
}
