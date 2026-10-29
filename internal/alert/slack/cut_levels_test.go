package slack

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// TestNoteBlocksCutLevels pins the blocks at every evidence cut level:
// each section stays within Slack's limit and the fix survives. Update
// with go test -run 'TestNoteBlocksCutLevels/level-1$' -update.
func TestNoteBlocksCutLevels(t *testing.T) {
	for _, level := range providertest.CutLevels {
		t.Run(fmt.Sprintf("level-%d", level), func(t *testing.T) {
			blocks := noteBlocks(providertest.CutLevel(level,
				maxSectionTextChars))
			body, err := json.MarshalIndent(blocks.BlockSet, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			first, _ := json.Marshal(blocks.BlockSet[0])
			if !strings.Contains(string(first), "--limits=memory=384Mi") {
				t.Fatalf("the fix was cut: %s", first)
			}
			providertest.CompareGolden(t, fmt.Sprintf(
				"testdata/cut/level-%d.golden", level), string(body)+"\n")
		})
	}
}
