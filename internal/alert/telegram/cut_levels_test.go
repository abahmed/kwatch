package telegram

import (
	"fmt"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// TestIncidentTextCutLevels pins the text at every evidence cut level.
// Update with go test -run 'TestIncidentTextCutLevels/level-1$' -update.
func TestIncidentTextCutLevels(t *testing.T) {
	for _, level := range providertest.CutLevels {
		t.Run(fmt.Sprintf("level-%d", level), func(t *testing.T) {
			text := incidentText(providertest.CutLevel(level,
				telegramTextLimit), telegramTextLimit)
			if len(text) > telegramTextLimit {
				t.Fatalf("text is %d bytes", len(text))
			}
			if !strings.Contains(text, providertest.FixCommand) {
				t.Fatalf("the fix was cut:\n%s", text)
			}
			providertest.CompareGolden(t, fmt.Sprintf(
				"testdata/cut/level-%d.golden", level), text+"\n")
		})
	}
}
