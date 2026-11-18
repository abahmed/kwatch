package email

import (
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func TestEvidenceMailShowsNotesAndCapsThem(t *testing.T) {
	m := providertest.ProEvidence()
	m.Evidence.Users = "Users are <still> served."
	m.Evidence.Grouped = "Grouped because all pods share node n1."
	m.Evidence.Replicas = []string{"r1", "r2", "r3"}
	m.Evidence.Checked = []string{"c1", "c2", "c3", "c4", "c5", "c6"}
	text, page := evidenceText(m), evidenceHTML(m)
	if !strings.Contains(page, "Users are &lt;still&gt; served.") {
		t.Fatalf("users line not escaped:\n%s", page)
	}
	for _, doc := range []string{text, page} {
		if strings.Contains(doc, "r3") || strings.Contains(doc, "c6") ||
			!strings.Contains(doc, "r2") || !strings.Contains(doc, "c5") {
			t.Fatalf("cap wrong:\n%s", doc)
		}
		u := strings.Index(doc, "Users are")
		if u < strings.Index(doc, "What kwatch found") ||
			u > strings.Index(doc, "Grouped") ||
			strings.Index(doc, "r1") > strings.Index(doc, "What kwatch checked") {
			t.Fatalf("order wrong:\n%s", doc)
		}
	}
	providertest.CompareGolden(t, "testdata/evidence/notes.txt.golden", text)
	providertest.CompareGolden(t, "testdata/evidence/notes.html.golden",
		page+"\n")
}

func TestEvidenceMailEmptyNotesAddNothing(t *testing.T) {
	m := providertest.ProEvidence()
	text, page := evidenceText(m), evidenceHTML(m)
	m.Evidence.Replicas, m.Evidence.Checked = []string{}, []string{}
	if evidenceText(m) != text || evidenceHTML(m) != page {
		t.Fatal("empty fields changed the mail")
	}
}
