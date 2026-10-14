package teams

import (
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// TestContract pins the exact Power Automate flow request for the
// canonical messages. Update with KWATCH_UPDATE_ALL=1 go test -run
// TestContract -update.
func TestContract(t *testing.T) {
	rec := providertest.NewRecorder(t)
	c := NewTeams(map[string]interface{}{"webhook": rec.URL()}, "dev",
		rec.Dependencies())
	providertest.RunContract(t, providertest.Contract{
		Chat: true,
		Send: c.SendIncident, Rec: rec, LimitBytes: 4000,
		Check: checkShape,
	})
}

// checkShape asserts a title, a text and one adaptive card whose body is
// the title block then the same text.
func checkShape(
	t *testing.T, name string, reqs []providertest.Request,
) {
	if len(reqs) != 1 || reqs[0].Method != "POST" {
		t.Fatalf("requests = %+v", reqs)
	}
	p, text := decodeFlow(t, reqs[0].Body)
	if strings.TrimSpace(p.Title) == "" || strings.TrimSpace(text) == "" ||
		text != p.Text {
		t.Fatalf("title/text mismatch: %s", reqs[0].Body)
	}
	card := p.Attachment[0]
	if card["contentType"] != "application/vnd.microsoft.card.adaptive" {
		t.Fatalf("card type = %v", card["contentType"])
	}
	providertest.AssertFencesBalanced(t, text)
}
