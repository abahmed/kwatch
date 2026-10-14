package mattermost

import (
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// TestContract pins the exact incoming-webhook request for the canonical
// messages. Update with KWATCH_UPDATE_ALL=1 go test -run TestContract
// -update.
func TestContract(t *testing.T) {
	rec := providertest.NewRecorder(t)
	c := NewMattermost(map[string]interface{}{"webhook": rec.URL()},
		"dev", rec.Dependencies())
	providertest.RunContract(t, providertest.Contract{
		Chat: true,
		Send: c.SendIncident, Rec: rec, LimitBytes: 4000,
		Check: checkShape,
	})
}

// checkShape asserts the Mattermost webhook shape: a non-empty text with
// balanced fences and one attachment carrying the cluster field.
func checkShape(
	t *testing.T, name string, reqs []providertest.Request,
) {
	if len(reqs) != 1 || reqs[0].Method != "POST" {
		t.Fatalf("requests = %+v", reqs)
	}
	body := reqs[0].JSON(t)
	text, _ := body["text"].(string)
	if strings.TrimSpace(text) == "" {
		t.Fatalf("text is empty: %s", reqs[0].Body)
	}
	providertest.AssertFencesBalanced(t, text)
	if strings.Contains(text, "@channel") ||
		strings.Contains(text, "@all") {
		t.Fatalf("mention not neutralized: %q", text)
	}
	att, _ := body["attachments"].([]any)
	if len(att) != 1 ||
		!strings.Contains(string(reqs[0].Body), `"Cluster"`) {
		t.Fatalf("cluster attachment missing: %s", reqs[0].Body)
	}
}
