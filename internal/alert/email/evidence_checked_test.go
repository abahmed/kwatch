package email

import (
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// What kwatch checked follows what it found, in both bodies, escaped
// in HTML; without checked items there is no heading.
func TestEvidenceMailSaysWhatWasChecked(t *testing.T) {
	m := providertest.ProEvidence()
	m.Evidence.Checked = []string{"kwatch checked: its node is healthy.",
		"kwatch cannot read <pod> logs here."}
	text, page := evidenceText(m), evidenceHTML(m)
	for _, doc := range []string{text, page} {
		found := strings.Index(doc, "What kwatch found")
		checked := strings.Index(doc, "What kwatch checked")
		if checked < 0 || checked < found {
			t.Fatalf("checked part missing or out of order:\n%s", doc)
		}
	}
	if !strings.Contains(text, "kwatch checked: its node is healthy.") ||
		!strings.Contains(page, "&lt;pod&gt;") {
		t.Fatalf("checked items missing or not escaped:\n%s\n%s",
			text, page)
	}
	m.Evidence.Checked = nil
	if strings.Contains(evidenceText(m)+evidenceHTML(m),
		"What kwatch checked") {
		t.Fatal("an empty checked part has a heading")
	}
}
