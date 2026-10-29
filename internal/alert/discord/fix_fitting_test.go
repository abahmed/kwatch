package discord

import (
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func TestIncidentContentKeepsFixWithinLimit(t *testing.T) {
	content := incidentContent(providertest.ProFix())
	if len(content) > maxContent {
		t.Fatalf("content is %d bytes, over %d", len(content), maxContent)
	}
	if !strings.Contains(content, providertest.FixCommand) {
		t.Fatalf("the fix was cut:\n%s", content)
	}
}
