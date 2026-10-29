package telegram

import (
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func TestIncidentTextKeepsFixWithinLimit(t *testing.T) {
	// A tighter limit than Telegram's own proves the cut leaves the fix.
	const limit = telegramTextLimit / 2
	text := incidentText(providertest.ProFix(), limit)
	if len(text) > limit {
		t.Fatalf("text is %d bytes, over %d", len(text), limit)
	}
	if !strings.Contains(text, providertest.FixCommand) {
		t.Fatalf("the fix was cut:\n%s", text)
	}
}
