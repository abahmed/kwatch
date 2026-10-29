package discord

import (
	"fmt"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// TestEmbedCutLevels pins the description at every evidence cut level:
// it fits, and the fix survives. Update with
// go test -run 'TestEmbedCutLevels/level-1$' -update.
func TestEmbedCutLevels(t *testing.T) {
	for _, level := range providertest.CutLevels {
		t.Run(fmt.Sprintf("level-%d", level), func(t *testing.T) {
			e := sendOne(t, providertest.CutLevel(level,
				maxEmbedDescription))
			if len(e.Description) > maxEmbedDescription ||
				embedSize(e) > maxEmbedTotal {
				t.Fatalf("embed is %d bytes", embedSize(e))
			}
			if !strings.Contains(e.Description, providertest.FixCommand) {
				t.Fatalf("the fix was cut:\n%s", e.Description)
			}
			providertest.CompareGolden(t, fmt.Sprintf(
				"testdata/cut/level-%d.golden", level),
				e.Title+"\n---\n"+e.Description+"\n")
		})
	}
}
