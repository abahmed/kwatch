package structured

import (
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func TestIncidentKeepsPlainNoteAndAddsMarkdown(t *testing.T) {
	m := providertest.Rich()
	got := NewIncident("prod", m)
	if got.Note != m.Note || strings.Contains(got.Note, "\n") {
		t.Fatalf("note must stay the plain paragraph: %q", got.Note)
	}
	if !strings.HasPrefix(got.Markdown, "🔴 **payments** in **shop**") {
		t.Fatalf("markdown = %q", got.Markdown)
	}
	if NewIncident("prod", providertest.Announce()).Markdown != "" {
		t.Fatal("a message without blocks has no markdown field")
	}
}
