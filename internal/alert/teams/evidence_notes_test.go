package teams

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func TestEvidenceCardShowsNotesAndCapsThem(t *testing.T) {
	m := providertest.ProEvidence()
	m.Evidence.Users = "Users are still served."
	m.Evidence.Grouped = "Grouped because all pods share node n1."
	m.Evidence.Replicas = []string{"r1", "r2", "r3"}
	m.Evidence.Checked = []string{"c1", "c2", "c3", "c4", "c5", "c6"}
	body, _ := json.Marshal(evidenceCardAttachments("payments", m))
	got := string(body)
	for _, want := range []string{"Users are still served.",
		"Grouped because", "r1", "r2", "c5"} {
		if !strings.Contains(got, want) {
			t.Fatalf("%q missing:\n%s", want, got)
		}
	}
	if strings.Contains(got, "r3") || strings.Contains(got, "c6") {
		t.Fatalf("cap not applied:\n%s", got)
	}
	if strings.Index(got, "Users are") > strings.Index(got, "Grouped") {
		t.Fatal("users line must come first")
	}
	providertest.CompareGolden(t, "testdata/evidence/card_notes.golden",
		got+"\n")
}

func TestEvidenceCardEmptyNotesAddNothing(t *testing.T) {
	m := providertest.ProEvidence()
	before, _ := json.Marshal(evidenceCardAttachments("p", m))
	m.Evidence.Replicas, m.Evidence.Checked = []string{}, []string{}
	after, _ := json.Marshal(evidenceCardAttachments("p", m))
	if string(before) != string(after) {
		t.Fatal("empty fields changed the card")
	}
}
