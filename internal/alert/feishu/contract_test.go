package feishu

import (
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// TestContract pins the exact signed interactive-card request for the
// canonical messages. Update with KWATCH_UPDATE_ALL=1 and -update.
func TestContract(t *testing.T) {
	rec := providertest.NewRecorder(t)
	c := NewFeiShu(map[string]interface{}{
		"webhook": rec.URL() + "/open-apis/bot/v2/hook/hk",
		"title":   "kwatch", "secret": "sig-secret",
	}, "dev", rec.Dependencies())
	providertest.RunContract(t, providertest.Contract{
		Chat: true,
		Send: c.SendIncident, Rec: rec,
		Secrets: []string{"hk", "sig-secret"}, LimitBytes: feiShuTextLimit,
		Check: checkCard,
	})
}

func checkCard(
	t *testing.T, name string, reqs []providertest.Request,
) {
	if len(reqs) != 1 || reqs[0].Method != "POST" {
		t.Fatalf("requests = %+v", reqs)
	}
	b := reqs[0].JSON(t)
	if b["msg_type"] != "interactive" || b["timestamp"] == "" ||
		b["sign"] == "" {
		t.Fatalf("card lacks msg_type or signature: %v", b)
	}
	card := b["card"].(map[string]any)
	title := card["header"].(map[string]any)["title"].(map[string]any)
	if title["tag"] != "plain_text" || title["content"] != "kwatch" {
		t.Fatalf("header title = %v", title)
	}
	el := card["elements"].([]any)[0].(map[string]any)
	text, _ := el["content"].(string)
	if el["tag"] != "markdown" || text == "" {
		t.Fatalf("element = %v", el)
	}
	if len(text) > feiShuTextLimit {
		t.Fatalf("content is %d bytes", len(text))
	}
	providertest.AssertFencesBalanced(t, text)
	if name == "markdown-code" && !strings.Contains(text,
		"`boom *x* @\u200bchannel`") {
		t.Fatalf("code span is not rendered as inline code: %q", text)
	}
}
