package delivery

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/notification"
)

func textJob(key, note string, output ...string) deliverJob {
	return deliverJob{kind: jobIncident, incident: &notification.Message{
		Key: key, Opens: true, Status: notification.StatusCritical,
		Note: note, Output: output}}
}

// The trail keeps the text of the latest message each provider accepted,
// and a failed send keeps a plain-language reason instead.
func TestSendTrailKeepsLatestSentTextPerProvider(t *testing.T) {
	chat := &errorRecorderProvider{name: "chat"}
	pager := &errorRecorderProvider{name: "pager"}
	am := newTestManager()
	setManagerEntries(am, []providerEntry{
		{provider: chat, retry: retryConfig{maxAttempts: 1}},
		{provider: pager, retry: retryConfig{maxAttempts: 1}},
	})
	entries := managerEntries(am)
	ctx := context.Background()

	_ = am.dispatch(ctx, &entries[0], textJob("k", "first"), deliverOpts{})
	_ = am.dispatch(ctx, &entries[0],
		textJob("k", "second", "log line"), deliverOpts{})
	pager.err = transport.Permanent(&transport.StatusError{
		Provider: "pager", StatusCode: 401})
	_ = am.dispatch(ctx, &entries[1], textJob("k", "third"), deliverOpts{})

	texts := am.SentTexts("k")
	require.Len(t, texts, 1, "only accepted sends are kept")
	assert.Equal(t, "second\n\nlog line", texts["chat"].Text)
	assert.Equal(t, "incident", texts["chat"].Kind)
	trail := am.SendTrail("k")
	require.Len(t, trail, 3)
	assert.Empty(t, trail[0].Reason)
	assert.Equal(t, "token refused (401)", trail[2].Reason)
	assert.Nil(t, am.SentTexts("other"))
}

func TestSentTextIsBounded(t *testing.T) {
	long := strings.Repeat("é", maxSentTextBytes)
	s := newSentText(time.Unix(1, 0), "incident", long)
	assert.LessOrEqual(t, len(s.Text), maxSentTextBytes)
	assert.True(t, s.Cut)
	assert.False(t, newSentText(time.Unix(1, 0), "", "short").Cut)
}

// A provider's sent text is saved with the trail and comes back after a
// restart; a text noted by the new session wins over the saved one.
func TestSendTrailSentTextSurvivesRestart(t *testing.T) {
	var trail sendTrail
	trail.noteText("k", "chat", SentText{Text: "old"})
	trail.noteText("k", "mail", SentText{Text: "mail"})
	trail.add("k", SendAttempt{Result: resultOK})
	trail.restore(nil)
	puts, _ := trail.take()
	require.Len(t, puts, 1)

	var next sendTrail
	next.noteText("k", "chat", SentText{Text: "new"})
	next.add("k", SendAttempt{Result: resultOK})
	next.restore(puts)

	next.mu.Lock()
	texts := next.textsLocked("k")
	next.mu.Unlock()
	assert.Equal(t, "new", texts["chat"].Text)
	assert.Equal(t, "mail", texts["mail"].Text)
}

// A record saved before texts and reasons existed still loads, and the
// new fields are left out of a record that has none.
func TestTrailRecordOldLayoutStillLoads(t *testing.T) {
	old := `{"version":1,"key":"k","attempts":[{"at":` +
		`"2026-01-02T03:04:05Z","result":"ok"}]}`
	var r TrailRecord
	require.NoError(t, json.Unmarshal([]byte(old), &r))
	var trail sendTrail
	trail.restore([]TrailRecord{r})
	require.Len(t, trail.of("k"), 1)

	out, err := json.Marshal(r)
	require.NoError(t, err)
	for _, field := range []string{"sent", "reason", "pager"} {
		assert.NotContains(t, string(out), `"`+field+`"`)
	}
}

// The bound forgets an incident's texts with its attempts.
func TestSendTrailBoundForgetsSentText(t *testing.T) {
	var trail sendTrail
	trail.noteText("first", "chat", SentText{Text: "x"})
	trail.add("first", SendAttempt{Result: resultOK})
	for i := 0; i < maxTrailKeys; i++ {
		trail.add(fmt.Sprintf("k%d", i), SendAttempt{Result: resultOK})
	}
	trail.mu.Lock()
	defer trail.mu.Unlock()
	assert.Nil(t, trail.textsLocked("first"))
	assert.Empty(t, trail.sent["first"])
}
