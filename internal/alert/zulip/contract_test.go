package zulip

import (
	"net/url"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// TestContract pins the exact messages API request for the canonical
// messages. Update with KWATCH_UPDATE_ALL=1 go test -run TestContract
// -update.
func TestContract(t *testing.T) {
	rec := providertest.NewRecorder(t)
	c := NewZulip(map[string]interface{}{
		"email": "kwatch@example.com", "token": "zulip-secret",
		"channel": "alerts", "url": rec.URL(),
	}, "dev", rec.Dependencies())
	if c == nil {
		t.Fatal("zulip was not constructed")
	}
	providertest.RunContract(t, providertest.Contract{
		Chat: true,
		Send: c.SendIncident, Rec: rec, LimitBytes: 10000,
		Headers: []string{"Authorization"},
		Secrets: []string{"zulip-secret"},
		Check:   checkShape,
	})
}

// checkShape asserts the stream-message form: type, to, subject and
// content, Basic auth, and balanced fences.
func checkShape(
	t *testing.T, name string, reqs []providertest.Request,
) {
	if len(reqs) != 1 || reqs[0].Method != "POST" ||
		reqs[0].Path != "/api/v1/messages" {
		t.Fatalf("requests = %+v", reqs)
	}
	form, err := url.ParseQuery(string(reqs[0].Body))
	if err != nil {
		t.Fatal(err)
	}
	if form.Get("type") != "stream" || form.Get("to") != "alerts" ||
		form.Get("subject") == "" {
		t.Fatalf("form = %v", form)
	}
	content := form.Get("content")
	if strings.TrimSpace(content) == "" {
		t.Fatal("content is empty")
	}
	providertest.AssertFencesBalanced(t, content)
	if !strings.HasPrefix(reqs[0].Header.Get("Authorization"), "Basic ") {
		t.Fatal("Basic auth missing")
	}
}
