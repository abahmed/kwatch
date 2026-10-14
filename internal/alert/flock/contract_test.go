package flock

import (
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// TestContract pins the exact webhook request for the canonical
// messages. Update with KWATCH_UPDATE_ALL=1 and -update.
func TestContract(t *testing.T) {
	rec := providertest.NewRecorder(t)
	c := NewFlock(map[string]interface{}{
		"webhook": rec.URL() + "/hooks/hk"}, "dev", rec.Dependencies())
	providertest.RunContract(t, providertest.Contract{
		Chat: true,
		Send: c.SendIncident, Rec: rec,
		Secrets: []string{"hk"}, Check: checkText,
	})
}

func checkText(
	t *testing.T, name string, reqs []providertest.Request,
) {
	if len(reqs) != 1 || reqs[0].Method != "POST" {
		t.Fatalf("requests = %+v", reqs)
	}
	text, _ := reqs[0].JSON(t)["text"].(string)
	if text == "" {
		t.Fatal("text is empty")
	}
	if strings.Contains(text, "```") && name != "near-limit" &&
		name != "markdown-code" {
		t.Fatalf("plain text carries markdown fences: %q", text)
	}
}
