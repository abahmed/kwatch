package teams

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// TestEvidenceCardLayout pins the Pro card: cause, what kwatch found,
// the monospace quote and the fix, with no action. Update with
// go test -run 'TestEvidenceCardLayout' -update (KWATCH_UPDATE_ALL=1).
func TestEvidenceCardLayout(t *testing.T) {
	m := providertest.ProEvidence()
	attachments := evidenceCardAttachments("payments", m)
	body, err := json.MarshalIndent(attachments, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got := string(body)
	if strings.Contains(got, "Action.") || strings.Contains(got, "@everyone") {
		t.Fatalf("the card has an action or a live mention:\n%s", got)
	}
	if !strings.Contains(got, "Monospace") ||
		!strings.Contains(got, "--limits=memory=384Mi") {
		t.Fatalf("quote or fix missing:\n%s", got)
	}
	providertest.CompareGolden(t, "testdata/evidence/card.golden", got+"\n")
}

func TestTeamsUsesEvidenceCardOnlyWithEvidence(t *testing.T) {
	tm := &Teams{}
	plain, err := tm.buildRequestBodyTeams(providertest.Announce())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(plain), "What kwatch found") {
		t.Fatal("a message without evidence got the evidence card")
	}
	pro, err := tm.buildRequestBodyTeams(providertest.ProEvidence())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(pro), "What kwatch found") {
		t.Fatal("a Pro message did not get the evidence card")
	}
}
