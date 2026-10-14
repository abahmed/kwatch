package googlechat

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
	c := NewGoogleChat(map[string]interface{}{"webhook": rec.URL()},
		"dev", rec.Dependencies())
	providertest.RunContract(t, providertest.Contract{
		Chat: true,
		Send: c.SendIncident, Rec: rec, LimitBytes: 4000,
		Check: checkShape,
	})
}

// checkShape asserts the Google Chat message shape: a JSON object whose
// only field is a non-empty text, with balanced code fences.
func checkShape(
	t *testing.T, name string, reqs []providertest.Request,
) {
	if len(reqs) != 1 || reqs[0].Method != "POST" {
		t.Fatalf("requests = %+v", reqs)
	}
	body := reqs[0].JSON(t)
	text, _ := body["text"].(string)
	if len(body) != 1 || strings.TrimSpace(text) == "" {
		t.Fatalf("body must be one non-empty text: %s", reqs[0].Body)
	}
	providertest.AssertFencesBalanced(t, text)
	if strings.Contains(text, "@channel") ||
		strings.Contains(text, "<users/all>") {
		t.Fatalf("mention not neutralized: %q", text)
	}
}
