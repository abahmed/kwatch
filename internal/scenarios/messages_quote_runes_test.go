package scenarios

import (
	"encoding/json"
	"strings"
	"testing"
)

// quoteRunes are the two control runes the composer wraps quotes in while
// it builds a sentence. They must be gone from every field of every
// message: a stray one would turn up as garbage, or split text past the
// redactor.
var quoteRunes = []string{`\u001e`, `\u001f`}

// TestScenarioMessagesHoldNoQuoteRunes checks every field of every message
// of every scenario (the JSON form lists them all, and escapes a control
// rune as \u001e or \u001f).
func TestScenarioMessagesHoldNoQuoteRunes(t *testing.T) {
	t.Parallel()
	for _, s := range library() {
		name := s.expect.Name
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, _, result := replayScenario(t, labelledDir, name)
			for _, m := range result.Messages {
				raw, err := json.Marshal(m)
				if err != nil {
					t.Fatal(err)
				}
				for _, mark := range quoteRunes {
					if strings.Contains(string(raw), mark) {
						t.Errorf("message holds %s: %s", mark, raw)
					}
				}
			}
		})
	}
}
