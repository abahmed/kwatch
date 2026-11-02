package rocketchat

import (
	"context"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

const userMentionText = "paged @alice and @devops-team; " +
	"ops@all-hands.io postgres://app@everyone-db:5432/x " +
	"git@here.example.com"

// RocketChat notifies any @name, so every name in workload text is broken,
// while e-mail addresses, URLs and DSNs reach the reader whole.
func TestRocketChatNeutralizesUserMentions(t *testing.T) {
	rec := providertest.NewRecorder(t)
	c := NewRocketChat(map[string]interface{}{"webhook": rec.URL()},
		"", rec.Dependencies())
	msg := providertest.Announce()
	msg.Note = "🔴 " + userMentionText
	msg.Output = []string{userMentionText}
	if err := c.SendIncident(context.Background(), msg); err != nil {
		t.Fatalf("SendIncident: %v", err)
	}
	if err := c.SendMessage(
		context.Background(), userMentionText,
	); err != nil {
		t.Fatalf("SendMessage: %v", err)
	}
	for _, r := range rec.Requests() {
		text, _ := r.JSON(t)["text"].(string)
		for _, live := range []string{"@alice", "@devops-team"} {
			if strings.Contains(text, live) {
				t.Fatalf("live mention %q in %q", live, text)
			}
		}
		for _, kept := range []string{"ops@all-hands.io",
			"app@everyone-db:5432", "git@here.example.com"} {
			if !strings.Contains(text, kept) {
				t.Fatalf("%q was altered in %q", kept, text)
			}
		}
	}
}
