package scorecard

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/audit"
)

func entry(ts, key, reason, action string) string {
	return `{"ts":"` + ts + `","incidentKey":"` + key +
		`","reason":"` + reason + `","action":"` + action + `"}`
}

func TestParseJSONLines(t *testing.T) {
	input := entry("2026-01-01T00:00:00Z", "ns:key1", "Test", "create") + "\n" +
		entry("2026-01-01T00:00:01Z", "ns:key1", "Test", "update")
	entries, err := Parse(strings.NewReader(input))
	require.NoError(t, err)
	require.Len(t, entries, 2)
	assert.Equal(t, "ns:key1", entries[0].IncidentKey)
	assert.Equal(t, audit.ActionCreate, entries[0].Action)
	assert.Equal(t, audit.ActionUpdate, entries[1].Action)
}

func TestParseSkipsNonAuditLines(t *testing.T) {
	input := `I0101 00:00:00.000000 log line
` + entry("2026-01-01T00:00:00Z", "ns:key1", "Test", "create") + `
malformed json
`
	entries, err := Parse(strings.NewReader(input))
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "ns:key1", entries[0].IncidentKey)
}

func TestParseSkipsEntriesWithoutAction(t *testing.T) {
	input := `{"ts":"2026-01-01T00:00:00Z","incidentKey":"ns:key1","reason":"Test"}
` + entry("2026-01-01T00:00:00Z", "ns:key1", "Test", "create")
	entries, err := Parse(strings.NewReader(input))
	require.NoError(t, err)
	require.Len(t, entries, 1)
}

func TestParseSkipsEntriesWithoutIncidentKey(t *testing.T) {
	input := `{"ts":"2026-01-01T00:00:00Z","reason":"Test","action":"create"}
` + entry("2026-01-01T00:00:00Z", "ns:key1", "Test", "create")
	entries, err := Parse(strings.NewReader(input))
	require.NoError(t, err)
	require.Len(t, entries, 1)
}

func TestParseSortsEntriesByTimestamp(t *testing.T) {
	input := entry("2026-01-01T00:00:03Z", "ns:k1", "Test", "create") + "\n" +
		entry("2026-01-01T00:00:01Z", "ns:k1", "Test", "create") + "\n" +
		entry("2026-01-01T00:00:02Z", "ns:k1", "Test", "create")
	entries, err := Parse(strings.NewReader(input))
	require.NoError(t, err)
	require.Len(t, entries, 3)
	assert.Less(t, entries[0].Timestamp, entries[1].Timestamp)
	assert.Less(t, entries[1].Timestamp, entries[2].Timestamp)
}

func TestParseEmptyInput(t *testing.T) {
	entries, err := Parse(strings.NewReader(""))
	require.NoError(t, err)
	require.Nil(t, entries)
}

func TestParseInvalidJSON(t *testing.T) {
	input := `{invalid json here}
`
	entries, err := Parse(strings.NewReader(input))
	require.NoError(t, err)
	require.Len(t, entries, 0)
}
