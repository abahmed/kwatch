package providertest

import (
	"net/http"
	"strings"
	"testing"
)

func TestUpdateScopeError(t *testing.T) {
	if updateScopeError("TestContract", "") == nil ||
		updateScopeError("TestContract/.*", "") == nil {
		t.Fatal("unscoped -update must be refused")
	}
	if updateScopeError("TestContract/resolve$", "") != nil ||
		updateScopeError("", "1") != nil {
		t.Fatal("scoped or deliberate -update must pass")
	}
}

func TestSnapshotRedactsAndNormalizes(t *testing.T) {
	h := http.Header{}
	h.Set("Content-Type", "application/json")
	h.Set("Authorization", "Bearer abc123")
	h.Set("X-Signature", "deadbeef")
	h.Set("X-Team", "ops")
	r := Request{
		Method: "POST", Path: "/v1/x", Query: "b=2&a=1&token=s3",
		Header: h,
		Body: []byte(`{"z":1,"a":"at 2026-01-02T03:04:05Z abc123",` +
			`"long":"` + strings.Repeat("y", 500) + `"}`),
	}
	got := Snapshot(r, SnapshotOptions{
		Headers: []string{"Authorization", "X-Signature", "X-Team"},
		Secrets: []string{"abc123"},
	})
	for _, want := range []string{
		"POST /v1/x?a=1&b=2&token=<redacted>",
		"Authorization: Bearer <redacted>",
		"X-Signature: <redacted>", "X-Team: ops",
		`"a": "at <time> <secret>"`, "500 runes sha256:",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("snapshot lacks %q:\n%s", want, got)
		}
	}
	if strings.Index(got, `"a"`) > strings.Index(got, `"z"`) {
		t.Fatal("JSON keys are not sorted")
	}
}

func TestNormalizeFormBody(t *testing.T) {
	got := NormalizeBody("application/x-www-form-urlencoded",
		[]byte("b=2&a=hello+world"))
	if got != "a=hello world\nb=2\n" {
		t.Fatalf("form = %q", got)
	}
}
