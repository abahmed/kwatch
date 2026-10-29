package email

import (
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// TestEvidenceMailLayout pins the Pro mail: the plain text and the HTML
// alternative carry the same parts in the same order, and log text is
// escaped. Update with KWATCH_UPDATE_ALL=1 go test
// -run TestEvidenceMailLayout -update.
func TestEvidenceMailLayout(t *testing.T) {
	m := providertest.ProEvidence()
	text, page := evidenceText(m), evidenceHTML(m)
	order := []string{"What kwatch found", "Log, container app",
		"Fix (changes the cluster)"}
	for _, doc := range []string{text, page} {
		at := -1
		for _, part := range order {
			i := strings.Index(doc, part)
			if i <= at {
				t.Fatalf("%q out of order in:\n%s", part, doc)
			}
			at = i
		}
	}
	if strings.Contains(page, "<@everyone>") {
		t.Fatal("log text was not escaped in HTML")
	}
	providertest.CompareGolden(t, "testdata/evidence/body.txt.golden", text)
	providertest.CompareGolden(t, "testdata/evidence/body.html.golden",
		page+"\n")
}
