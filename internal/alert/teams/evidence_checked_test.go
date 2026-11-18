package teams

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func TestEvidenceCardSaysWhatWasChecked(t *testing.T) {
	m := providertest.ProEvidence()
	m.Evidence.Checked = []string{"kwatch checked: its node is healthy."}
	body, err := json.Marshal(evidenceCardAttachments("payments", m))
	if err != nil {
		t.Fatal(err)
	}
	got := string(body)
	if !strings.Contains(got, "What kwatch checked") ||
		!strings.Contains(got, "kwatch checked: its node is healthy.") {
		t.Fatalf("checked part missing:\n%s", got)
	}
	m.Evidence.Checked = nil
	body, _ = json.Marshal(evidenceCardAttachments("payments", m))
	if strings.Contains(string(body), "What kwatch checked") {
		t.Fatal("an empty checked part has a heading")
	}
}
