package teamsworkflow

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// TestContract pins the exact Workflows webhook request for the
// canonical messages. Update with KWATCH_UPDATE_ALL=1 go test -run
// TestContract -update.
func TestContract(t *testing.T) {
	rec := providertest.NewRecorder(t)
	c := NewTeamsWorkflow(map[string]interface{}{"webhook": rec.URL()},
		"dev", rec.Dependencies())
	providertest.RunContract(t, providertest.Contract{
		Chat: true,
		Send: c.SendIncident, Rec: rec, LimitBytes: 4000,
		Check: checkShape,
	})
}

// checkShape asserts the message envelope holds one adaptive card with
// one non-empty wrapped TextBlock.
func checkShape(
	t *testing.T, name string, reqs []providertest.Request,
) {
	if len(reqs) != 1 || reqs[0].Method != "POST" {
		t.Fatalf("requests = %+v", reqs)
	}
	var p teamsPayload
	if err := json.Unmarshal(reqs[0].Body, &p); err != nil {
		t.Fatal(err)
	}
	if p.Type != "message" || len(p.Attachments) != 1 {
		t.Fatalf("envelope = %s", reqs[0].Body)
	}
	a := p.Attachments[0]
	if a.ContentType != "application/vnd.microsoft.card.adaptive" ||
		a.Content.Type != "AdaptiveCard" || a.Content.Version == "" ||
		len(a.Content.Body) != 1 {
		t.Fatalf("card = %s", reqs[0].Body)
	}
	b := a.Content.Body[0]
	if b.Type != "TextBlock" || !b.Wrap || strings.TrimSpace(b.Text) == "" {
		t.Fatalf("text block = %+v", b)
	}
	providertest.AssertFencesBalanced(t, b.Text)
}
