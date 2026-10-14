package webex

import (
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// TestContract pins the exact create-message request for the canonical
// messages. Update with KWATCH_UPDATE_ALL=1 and -update.
func TestContract(t *testing.T) {
	c, rec := newRecordedWebex(t)
	providertest.RunContract(t, providertest.Contract{
		Chat: true,
		Send: c.SendIncident, Rec: rec,
		Headers: []string{"Authorization"},
		Secrets: []string{"token"}, LimitBytes: 7000,
		Check: checkMessage,
	})
}

func checkMessage(
	t *testing.T, name string, reqs []providertest.Request,
) {
	if len(reqs) != 1 || reqs[0].Method != "POST" {
		t.Fatalf("requests = %+v", reqs)
	}
	if reqs[0].Header.Get("Authorization") != "Bearer token" {
		t.Fatal("missing bearer token")
	}
	b := reqs[0].JSON(t)
	text, _ := b["markdown"].(string)
	if b["roomId"] != "room-1" || text == "" {
		t.Fatalf("message lacks roomId or markdown: %v", b)
	}
	if len(text) > webexMarkdownLimit {
		t.Fatalf("markdown is %d bytes", len(text))
	}
	providertest.AssertFencesBalanced(t, text)
}
