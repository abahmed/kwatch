package discord

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// toRecorder sends every request to the recorder, so the real
// discordgo request path (discord.com host) is captured unchanged.
type toRecorder struct {
	base http.RoundTripper
	host *url.URL
}

func (r toRecorder) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.URL.Scheme, req.URL.Host = r.host.Scheme, r.host.Host
	return r.base.RoundTrip(req)
}

// TestContract pins the exact webhook execute request for the canonical
// messages. Update with KWATCH_UPDATE_ALL=1 go test -run TestContract
// -update.
func TestContract(t *testing.T) {
	rec := providertest.NewRecorder(t)
	host, _ := url.Parse(rec.URL())
	deps := rec.Dependencies()
	deps.HTTPClient = &http.Client{Transport: toRecorder{
		base: rec.Server.Client().Transport, host: host}}
	c := NewDiscord(map[string]interface{}{
		"webhook": "https://discord.com/api/webhooks/123/wh-secret",
	}, "dev", deps)
	if c == nil {
		t.Fatal("discord was not constructed")
	}
	providertest.RunContract(t, providertest.Contract{
		Chat: true,
		Send: c.SendIncident, Rec: rec, LimitBytes: maxContent,
		Secrets: []string{"wh-secret"},
		Check:   checkShape,
	})
}

// checkShape asserts the execute-webhook body: content within the 2000
// character limit, balanced fences, and mentions disabled.
func checkShape(
	t *testing.T, name string, reqs []providertest.Request,
) {
	if len(reqs) != 1 || reqs[0].Method != "POST" ||
		!strings.HasPrefix(reqs[0].Path, "/api/v9/webhooks/123/") {
		t.Fatalf("requests = %+v", reqs)
	}
	body := reqs[0].JSON(t)
	content, _ := body["content"].(string)
	if strings.TrimSpace(content) == "" {
		t.Fatalf("content is empty: %s", reqs[0].Body)
	}
	if n := utf8.RuneCountInString(content); n > maxContent {
		t.Fatalf("content is %d characters, limit %d", n, maxContent)
	}
	providertest.AssertFencesBalanced(t, content)
	am, _ := body["allowed_mentions"].(map[string]any)
	if parse, ok := am["parse"].([]any); !ok || len(parse) != 0 {
		t.Fatalf("mentions must be disabled: %s", reqs[0].Body)
	}
}
