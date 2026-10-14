package telegram

import (
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// TestContract pins the exact sendMessage request for the canonical
// messages. Update with KWATCH_UPDATE_ALL=1 and -update.
func TestContract(t *testing.T) {
	c, rec := recorderTelegram(t)
	providertest.RunContract(t, providertest.Contract{
		Chat: true,
		Send: c.SendIncident, Rec: rec,
		Secrets: []string{"tok"}, LimitBytes: telegramTextLimit,
		Check: checkSendMessage,
	})
}

func checkSendMessage(
	t *testing.T, name string, reqs []providertest.Request,
) {
	if len(reqs) != 1 || reqs[0].Method != "POST" {
		t.Fatalf("requests = %+v", reqs)
	}
	if reqs[0].Path != "/bottok/sendMessage" {
		t.Fatalf("path = %q", reqs[0].Path)
	}
	body := reqs[0].JSON(t)
	text, _ := body["text"].(string)
	if body["chat_id"] != "chat" || text == "" ||
		body["parse_mode"] != "HTML" {
		t.Fatalf("sendMessage lacks required fields: %v", body)
	}
	if len(text) > telegramTextLimit {
		t.Fatalf("text is %d bytes", len(text))
	}
	if strings.Count(text, "<pre>") != strings.Count(text, "</pre>") {
		t.Fatalf("unbalanced <pre> in %.200q", text)
	}
	if strings.Count(text, "<b>") != strings.Count(text, "</b>") {
		t.Fatalf("unbalanced <b> in %.200q", text)
	}
}
