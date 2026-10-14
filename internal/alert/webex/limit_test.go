package webex

import (
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
	"github.com/abahmed/kwatch/internal/notification"
)

// hugeIncident is a message with a note and output far over any limit.
func hugeIncident() notification.Message {
	m := providertest.Announce()
	m.Note = strings.Repeat("payments keeps crashing. ", 1000)
	m.Doc = nil
	m.Output = []string{strings.Repeat("panic: out of memory\n", 2000)}
	return m
}

func TestIncidentIsCutToWebexMarkdownLimit(t *testing.T) {
	c, rec := newRecordedWebex(t)
	if err := c.SendIncident(t.Context(), hugeIncident()); err != nil {
		t.Fatal(err)
	}
	reqs := rec.Requests()
	if len(reqs) != 1 {
		t.Fatalf("requests = %d", len(reqs))
	}
	text, _ := reqs[0].JSON(t)["markdown"].(string)
	if n := len(text); n == 0 || n > webexMarkdownLimit {
		t.Fatalf("markdown is %d bytes, limit %d", n, webexMarkdownLimit)
	}
	providertest.AssertFencesBalanced(t, text)
}

func TestPlainMessageIsCutToWebexMarkdownLimit(t *testing.T) {
	c, rec := newRecordedWebex(t)
	long := strings.Repeat("x", 3*webexMarkdownLimit)
	if err := c.SendMessage(t.Context(), long); err != nil {
		t.Fatal(err)
	}
	text, _ := rec.Requests()[0].JSON(t)["markdown"].(string)
	if len(text) > webexMarkdownLimit {
		t.Fatalf("markdown is %d bytes", len(text))
	}
}
