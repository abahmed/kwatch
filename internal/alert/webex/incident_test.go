package webex

import (
	"context"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func newRecordedWebex(t *testing.T) (*Webex, *providertest.Recorder) {
	t.Helper()
	rec := providertest.NewRecorder(t)
	w := NewWebex(map[string]interface{}{
		"accessToken": "token", "roomId": "room-1",
	}, "dev", rec.Dependencies())
	w.url = rec.URL()
	return w, rec
}

func TestWebexSendIncidentPostsNoteAsMarkdown(t *testing.T) {
	w, rec := newRecordedWebex(t)
	for _, tc := range providertest.Lifecycle() {
		t.Run(tc.Name, func(t *testing.T) {
			if err := w.SendIncident(
				context.Background(), tc.Message,
			); err != nil {
				t.Fatalf("SendIncident: %v", err)
			}
			req := rec.Last(t)
			body := req.JSON(t)
			text, _ := body["markdown"].(string)
			if !strings.HasPrefix(text, tc.Message.Note) {
				t.Fatalf("markdown = %q, want the Note first", text)
			}
			providertest.AssertOneLeadingEmoji(t, text)
			if body["roomId"] != "room-1" ||
				req.Header.Get("Authorization") != "Bearer token" {
				t.Fatalf("request = %+v", body)
			}
		})
	}
}

func TestWebexSendIncidentNeutralizesMentions(t *testing.T) {
	w, rec := newRecordedWebex(t)
	if err := w.SendIncident(
		context.Background(), providertest.Hostile(),
	); err != nil {
		t.Fatalf("SendIncident: %v", err)
	}
	text, _ := rec.Last(t).JSON(t)["markdown"].(string)
	if strings.Contains(text, "@channel") {
		t.Fatalf("mention was not neutralized: %q", text)
	}
}

func TestWebexSendIncidentNeutralizesWebexMentions(t *testing.T) {
	w, rec := newRecordedWebex(t)
	m := providertest.Announce()
	m.Output = []string{"<@all> <@personEmail:ops@example.com>"}
	if err := w.SendIncident(context.Background(), m); err != nil {
		t.Fatalf("SendIncident: %v", err)
	}
	text, _ := rec.Last(t).JSON(t)["markdown"].(string)
	if strings.Contains(text, "<@") {
		t.Fatalf("webex mention was not neutralized: %q", text)
	}
}
