package discord

import (
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func TestEmbedKeepsFixWithinLimit(t *testing.T) {
	e := sendOne(t, providertest.ProFix())
	if len(e.Description) > maxEmbedDescription ||
		embedSize(e) > maxEmbedTotal {
		t.Fatalf("embed is %d bytes", embedSize(e))
	}
	if !strings.Contains(e.Description, providertest.FixCommand) {
		t.Fatalf("the fix was cut:\n%s", e.Description)
	}
}
