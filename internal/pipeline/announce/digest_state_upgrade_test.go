package announce

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A digest state saved by rc.17 has no record of what the digests
// listed today. It restores with an empty record, so the first digest
// lists the ongoing problems in full, as it did before.
func TestDigestStateWithoutShownRestoresEmpty(t *testing.T) {
	var state DigestState
	require.NoError(t, json.Unmarshal([]byte(
		`{"Since":"2026-10-01T00:00:00Z","Listed":["a"]}`), &state))
	assert.Empty(t, state.Shown)

	c, _ := foldHarness(demotedRecord("a"))
	c.Startup.Summary.Digest = state
	c.RestoreDigest()

	assert.Empty(t, c.shownLines())
	changed, unchanged := c.splitOngoing(c.ongoingProblems(foldNow),
		foldNow.Add(time.Hour))
	assert.Len(t, changed, 1, "nothing was listed yet, so it is news")
	assert.Empty(t, unchanged)
}
