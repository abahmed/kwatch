package announce

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/notification"
)

// gapText is a gap report as the engine writes it.
const gapText = "kwatch was not running from 08:00 to 08:30 UTC (30m). " +
	"Compared with before, nothing changed and nothing crashed."

// A short restart with nothing new or worse says nothing, however
// often kwatch restarts: the incidents were told before and are
// unchanged. Only the audit log records each restart.
func TestQuietRestartSendsNothing(t *testing.T) {
	r := &restartRig{t: t}
	for i := range 6 {
		sent := r.start(foldNow.Add(time.Duration(i)*time.Hour), apiLost)
		assert.Empty(t, sent, "restart %d", i)
		assert.Empty(t, r.notified, "restart %d", i)
		require.Len(t, r.audit, 1, "restart %d is audited", i)
		assert.Equal(t, CarrierQuietRestart, r.audit[0].Carrier)
		require.NotNil(t, r.audit[0].Listed)
		assert.Len(t, r.audit[0].Listed.Items, 2)
	}
}

// After a long downtime the gap message is the one message of the
// restart: it waits for the restored incidents and counts the ones
// unchanged instead of listing them.
func TestRestartGapMessageCountsTheUnchanged(t *testing.T) {
	r := &restartRig{t: t, gap: gapText, issues: "https://k.example.com"}

	sent := r.start(foldNow, apiLost)

	assert.Empty(t, sent, "no restored listing beside the gap message")
	require.Len(t, r.notified, 1)
	assert.Equal(t, gapText+"\n\n2 ongoing issues unchanged; see the "+
		"dashboard: https://k.example.com/issues", r.notified[0])
}

// A restored incident nobody heard of is named; the one told and
// unchanged is only counted, and its tier (a page) does not make the
// message a page. The gap goes into the same message.
func TestRestartNamesOnlyNewsAtItsOwnTier(t *testing.T) {
	r := &restartRig{t: t, gap: gapText,
		unheard: map[string]bool{"web": true},
		tiers:   map[string]incident.Tier{"api": incident.Page}}

	sent := r.start(foldNow, apiLost)

	require.Len(t, sent, 1)
	assert.Empty(t, r.notified, "the gap rides in the one message")
	msg := sent[0]
	assert.NotEqual(t, notification.StatusCritical, msg.Status)
	assert.NotEqual(t, "critical", msg.Route.Severity)
	require.NotNil(t, msg.Listed)
	require.Len(t, msg.Listed.Items, 2)
	assert.True(t, strings.HasPrefix(msg.Listed.Items[0], "inc-web"))
	assert.Contains(t, msg.Listed.Items[1], "inc-api")
	assert.Contains(t, msg.Listed.Items[1], "counted")
	assert.Contains(t, msg.Note, "1 ongoing issue unchanged.")
	assert.Contains(t, msg.Note, "kwatch was not running from 08:00")
	assert.Contains(t, docText(msg.Doc), "1 ongoing issue unchanged.")

	again := r.start(foldNow.Add(time.Hour), apiLost)
	assert.Empty(t, again, "named once: the next restart counts it")
}

// The marker gained Restored and Spoke later: a marker written before
// them loads, and a marker with them survives the store's JSON.
func TestStartupStateMarksRoundTrip(t *testing.T) {
	var old StartupState
	require.NoError(t, json.Unmarshal([]byte(`{"Complete":true}`), &old))
	assert.Nil(t, old.Restored)
	assert.Nil(t, old.Spoke)

	in := StartupState{Complete: true, Restored: &RestoredMark{
		At: foldNow, IDs: []string{"a"}, NoteAt: foldNow, NoteKey: "k"},
		Spoke: map[string]string{"a": "d-a"}}
	raw, err := json.Marshal(in)
	require.NoError(t, err)
	var out StartupState
	require.NoError(t, json.Unmarshal(raw, &out))
	assert.Equal(t, in, out)

	clone := in.Clone()
	clone.Restored.IDs[0] = "changed"
	clone.Spoke["a"] = "changed"
	assert.Equal(t, "a", in.Restored.IDs[0], "Clone copies the mark")
	assert.Equal(t, "d-a", in.Spoke["a"], "Clone copies Spoke")
}

// docText joins the text of a rich document.
func docText(doc []notification.Block) string {
	var b strings.Builder
	for _, block := range doc {
		for _, span := range block.Spans {
			b.WriteString(span.Text)
		}
		b.WriteString("\n")
	}
	return b.String()
}
