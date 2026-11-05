package replay_test

import (
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/replay"
)

// scrubbing returns a sanitizer that keeps free text scrubbed, and the
// pseudonyms it gave a pod "api-7d9f" in "shop" and a Service "ledger".
func scrubbing(t *testing.T) (*replay.Sanitizer, string, string, string) {
	t.Helper()
	s := mustSanitizer(t, replay.SanitizeOptions{Salt: "salt",
		ScrubText: true})
	pod := s.Observation(inventory.Observation{
		Entity: inventory.EntityID{Kind: "pod", Namespace: "shop",
			Name: "api-7d9f"}})
	svc := s.Observation(inventory.Observation{
		Entity: inventory.EntityID{Kind: "service", Namespace: "shop",
			Name: "ledger"}})
	return s, pod.Entity.Name, pod.Entity.Namespace, svc.Entity.Name
}

func scrubbed(s *replay.Sanitizer, name, text string) string {
	out := s.Observation(inventory.Observation{
		Entity: inventory.EntityID{Kind: "pod", Namespace: "x", Name: "y"},
		Attributes: map[string]inventory.Value{
			name: inventory.Text(text)},
	})
	return out.Attributes[name].AsText()
}

// A name the log hid gets the same pseudonym inside a message, so the
// message still points at the object it names.
func TestScrubbedTextUsesTheObjectsPseudonyms(t *testing.T) {
	s, pod, ns, svc := scrubbing(t)
	got := scrubbed(s, "last.error.line",
		"panic: dial tcp ledger.shop.svc.cluster.local:9000: connect: "+
			"connection refused (pod api-7d9f)")
	want := "panic: dial tcp " + svc + "." + ns +
		".svc.cluster.local:9000: connect: connection refused (pod " +
		pod + ")"
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

// Credentials, outside hosts, emails and images are hidden; the words
// that say what failed stay.
func TestScrubbedTextHidesWhatIdentifies(t *testing.T) {
	s, _, _, _ := scrubbing(t)
	got := scrubbed(s, "message",
		"Unhandled exception. app.ConfigError: license must be "+
			"specified, see https://docs.vendor-name.com/license; "+
			"contact owner@corp-name.com; image "+
			"registry.corp-name.io/team/app:2.3; password=hunter2hunter2")
	for _, gone := range []string{"vendor-name", "corp-name", "owner@",
		"team/app", "hunter2hunter2"} {
		if strings.Contains(got, gone) {
			t.Errorf("%q kept in %q", gone, got)
		}
	}
	for _, kept := range []string{"Unhandled exception.",
		"app.ConfigError", "license must be specified", "/license"} {
		if !strings.Contains(got, kept) {
			t.Errorf("%q lost from %q", kept, got)
		}
	}
}

// Kubernetes' own domains and versions stay; a long text is cut.
func TestScrubbedTextKeepsSystemDomainsAndIsBounded(t *testing.T) {
	s, _, _, _ := scrubbing(t)
	taint := "0/3 nodes are available: 3 node(s) had untolerated " +
		"taint {node.kubernetes.io/not-ready: }, version 1.29.4"
	if got := scrubbed(s, "message", taint); got != taint {
		t.Errorf("got %q", got)
	}
	long := scrubbed(s, "message", strings.Repeat("word ", 400))
	if len(long) > 520 || !strings.HasSuffix(long, "…") {
		t.Errorf("not bounded: %d bytes", len(long))
	}
}

// Event messages and change causes are kept scrubbed; field values never
// are, since an edited setting may hold a secret.
func TestScrubbedTextCoversNotesButNotFieldValues(t *testing.T) {
	s, _, _, svc := scrubbing(t)
	out := s.Observation(inventory.Observation{
		Entity: inventory.EntityID{Kind: "pod", Namespace: "shop",
			Name: "api-7d9f"},
		Note: inventory.Note{Reason: "Unhealthy",
			Message: "Readiness probe failed: HTTP probe failed with " +
				"statuscode: 500"},
		Change: inventory.Change{Cause: "rolled back ledger",
			Fields: []inventory.FieldChange{{Path: "spec.env[TOKEN]",
				Before: "old value here", After: "new value here"}}},
	})
	if out.Note.Message != "Readiness probe failed: HTTP probe failed "+
		"with statuscode: 500" {
		t.Errorf("note %q", out.Note.Message)
	}
	if out.Change.Cause != "rolled back "+svc {
		t.Errorf("cause %q", out.Change.Cause)
	}
	f := out.Change.Fields[0]
	if strings.Contains(f.Before+f.After, "value") {
		t.Errorf("field values kept: %+v", f)
	}
}

// Without ScrubText, free text is dropped as before.
func TestFreeTextIsDroppedWithoutScrubbing(t *testing.T) {
	s := mustSanitizer(t, replay.SanitizeOptions{Salt: "salt"})
	out := s.Observation(inventory.Observation{
		Entity: inventory.EntityID{Kind: "pod", Namespace: "shop",
			Name: "api"},
		Attributes: map[string]inventory.Value{
			"last.error.line": inventory.Text("panic: boom now")},
		Note: inventory.Note{Message: "Back-off restarting"},
	})
	if _, ok := out.Attributes["last.error.line"]; ok ||
		out.Note.Message != "" {
		t.Errorf("free text kept: %+v %q", out.Attributes,
			out.Note.Message)
	}
}

// Names the log never saw are hidden after a Kubernetes kind, and names
// next to "_", in another case or in outside hosts of any shape are
// hidden too; the words that say what failed stay.
func TestScrubbedTextHidesUnknownAndOddlyWrittenNames(t *testing.T) {
	s, _, _, _ := scrubbing(t)
	for _, c := range []struct{ text, gone string }{
		{`configmap "billing-config" not found`, "billing-config"},
		{`secret 'db-creds' is missing`, "db-creds"},
		{"persistentvolumeclaim/data-orders-0 is pending",
			"data-orders-0"},
		{"Secret billing-token not found in namespace shop-2",
			"billing-token"},
		{"Secret billing-token not found in namespace shop-2", "shop-2"},
		{`deployments.apps "billing-api" not found`, "billing-api"},
		{"pvc data-1 lost", "data-1"},
		{"mounting volume_billing-api failed", "billing-api"},
		{"lookup DB01.Corp.Example.COM: no such host", "Corp"},
		{"dial tcp db-1.corp-internal:5432: i/o timeout",
			"corp-internal"},
		{"dial tcp other-svc.other-ns.svc.cluster.local:80: refused",
			"other-svc"},
		{"dial tcp other-svc.other-ns.svc.cluster.local:80: refused",
			"other-ns"},
		{"connect to Corp.Example.Com failed", "Example"},
	} {
		got := scrubbed(s, "message", c.text)
		if strings.Contains(strings.ToLower(got), strings.ToLower(c.gone)) {
			t.Errorf("%q kept in %q", c.gone, got)
		}
	}
	for _, kept := range []string{
		"exec /usr/bin/sh: exec format error",
		"dial tcp 10.0.0.5:5432: connect: connection refused",
		"x509: certificate has expired or is not yet valid",
		"Readiness probe failed: HTTP probe failed with statuscode: 500",
		"Unhandled exception. System.InvalidOperationException: boom",
		"java.lang.Error: boom at Main.run(Main.java:12) in main.go",
		"pod is pending, node had taint, version v1.29.4 after 2.5s",
		"0/3 nodes are available: 3 node(s) had untolerated taint " +
			"{node.kubernetes.io/not-ready: }",
	} {
		if got := scrubbed(s, "message", kept); got != kept {
			t.Errorf("changed:\n got  %q\n want %q", got, kept)
		}
	}
}

// A name inside text maps to the pseudonym of the object whatever its
// case, and whether the object comes before the text or with it.
func TestScrubbedTextMatchesObjectsInAnyCaseAndOrder(t *testing.T) {
	s, _, _, svc := scrubbing(t)
	if got := scrubbed(s, "message", "Ledger is down"); got !=
		svc+" is down" {
		t.Errorf("case: %q", got)
	}
	out := s.Observation(inventory.Observation{
		Entity: inventory.EntityID{Kind: "endpoints", Namespace: "shop",
			Name: "x"},
		Targets: []inventory.EntityID{{Kind: "service",
			Namespace: "shop", Name: "svc-new-zzz"}},
		Attributes: map[string]inventory.Value{
			"status.message": inventory.Text("waiting on svc-new-zzz"),
		},
	})
	got := out.Attributes["status.message"].AsText()
	if strings.Contains(got, "svc-new-zzz") ||
		got != "waiting on "+out.Targets[0].Name {
		t.Errorf("own target: %q (target %q)", got, out.Targets[0].Name)
	}
	// An object first named in text, then seen: one pseudonym.
	first := scrubbed(s, "message", `configmap "later-cfg" not found`)
	seen := s.Observation(inventory.Observation{
		Entity: inventory.EntityID{Kind: "configmap", Namespace: "shop",
			Name: "later-cfg"}})
	if !strings.Contains(first, seen.Entity.Name) {
		t.Errorf("text %q, object %q", first, seen.Entity.Name)
	}
}

// Capitalized hosts are no code, a known name inside a dotted token is
// hidden, multi-part file names stay, and a namespace named like a
// common word does not rewrite that word (found by a review).
func TestScrubbedTextCodeHostsFilesAndCommonWords(t *testing.T) {
	s, _, _, _ := scrubbing(t)
	s.Observation(inventory.Observation{Entity: inventory.EntityID{
		Kind: "namespace", Name: "default"}})
	for _, c := range []struct{ text, gone string }{
		{"connect to db.Prod.acme.xyz failed", "acme"},
		{"GET https://Billing.Example.Fr/api failed", "Billing"},
		{"resolve ledger.Shop.Example failed", "ledger"},
	} {
		got := scrubbed(s, "message", c.text)
		if strings.Contains(strings.ToLower(got), strings.ToLower(c.gone)) {
			t.Errorf("%q kept in %q", c.gone, got)
		}
	}
	for _, kept := range []string{
		"cannot read config.prod.yaml: no such file",
		"using default value for timeout",
		"at Main.run(Main.java:12)",
	} {
		if got := scrubbed(s, "message", kept); got != kept {
			t.Errorf("changed:\n got  %q\n want %q", got, kept)
		}
	}
}
