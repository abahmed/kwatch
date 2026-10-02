package slack

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	slackClient "github.com/slack-go/slack"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// tokenSlack talks to an httptest Slack Web API.
func tokenSlack(t *testing.T) (*Slack, *providertest.Recorder) {
	t.Helper()
	rec := providertest.NewRecorder(t)
	rec.Reply = func(w http.ResponseWriter, _ providertest.Request) {
		_, _ = w.Write([]byte(`{"ok":true,"channel":"C1","ts":"111.1"}`))
	}
	s := NewSlack(map[string]interface{}{
		"token": "xoxb-test", "channel": "C1",
	}, "dev", rec.Dependencies())
	require.NotNil(t, s)
	s.apiClient = slackClient.New("xoxb-test",
		slackClient.OptionHTTPClient(rec.Server.Client()),
		slackClient.OptionAPIURL(rec.URL()+"/"))
	return s, rec
}

type slackCall struct {
	method   string
	threadTS string
	text     string
}

func decodeCall(t *testing.T, req providertest.Request) slackCall {
	t.Helper()
	form, err := url.ParseQuery(string(req.Body))
	require.NoError(t, err)
	var blocks []struct {
		Text struct {
			Text string `json:"text"`
		} `json:"text"`
	}
	require.NoError(t, json.Unmarshal([]byte(form.Get("blocks")), &blocks))
	require.NotEmpty(t, blocks)
	return slackCall{
		method:   strings.TrimPrefix(req.Path, "/"),
		threadTS: form.Get("thread_ts"), text: blocks[0].Text.Text,
	}
}

func TestSlackTokenIncidentLifecycleThreadsByKey(t *testing.T) {
	s, rec := tokenSlack(t)
	want := map[string][]slackCall{
		"announce": {
			{"chat.postMessage", "", providertest.Announce().Short},
			{"chat.postMessage", "111.1", providertest.Announce().Note},
		},
		"update": {
			{"chat.postMessage", "111.1", providertest.Update().Note},
			{"chat.update", "", providertest.Update().Short},
		},
		"resolve": {
			{"chat.postMessage", "111.1", providertest.Resolve().Note},
			{"chat.update", "", providertest.Resolve().Short},
		},
	}
	for _, tc := range providertest.Lifecycle() {
		rec.Reset()
		require.NoError(t, s.SendIncident(context.Background(), tc.Message))
		requests := rec.Requests()
		require.Len(t, requests, len(want[tc.Name]), tc.Name)
		for i, req := range requests {
			call := decodeCall(t, req)
			require.Equal(t, want[tc.Name][i], call, tc.Name)
			providertest.AssertOneLeadingEmoji(t, call.text)
		}
	}
	require.Empty(t, s.SnapshotThreads(), "resolve drops the thread")
}

func TestSlackWebhookIncidentPostsNote(t *testing.T) {
	rec := providertest.NewRecorder(t)
	s := NewSlack(map[string]interface{}{"webhook": rec.URL()}, "dev",
		rec.Dependencies())
	for _, tc := range providertest.Lifecycle() {
		require.NoError(t, s.SendIncident(context.Background(), tc.Message))
		var msg slackClient.WebhookMessage
		require.NoError(t, json.Unmarshal(rec.Last(t).Body, &msg))
		text := firstText(msg.Blocks)
		require.Equal(t, tc.Message.Note, text, tc.Name)
		require.Equal(t, tc.Message.Short, msg.Text, "text fallback")
		providertest.AssertOneLeadingEmoji(t, text)
	}
}

func TestSlackIncidentEscapesMarkupAndMentions(t *testing.T) {
	blocks := noteBlocks(providertest.Hostile())
	text := firstText(blocks)
	require.NotContains(t, text, "<script>")
	require.Contains(t, text, "&lt;b&gt;pay&lt;/b&gt; &amp;")
	require.NotContains(t, text, "@channel")
	providertest.AssertOneLeadingEmoji(t, text)
}

func TestSlackIncidentTruncatesLongNote(t *testing.T) {
	m := providertest.Announce()
	m.Note += strings.Repeat(" é", maxSectionTextChars)
	text := firstText(noteBlocks(m))
	require.True(t, strings.HasSuffix(text, "..."))
	require.Equal(t, text, firstText(noteBlocks(m)), "deterministic")
}

func TestSlackCompactIncidentSendsShort(t *testing.T) {
	s := newTestSlack(map[string]interface{}{
		"webhook": "https://hooks.example.test/x", "compact": true,
	}, "dev")
	var text string
	s.send = func(_ string, msg *slackClient.WebhookMessage) error {
		text = msg.Text
		return nil
	}

	require.NoError(t, s.SendIncident(
		context.Background(), providertest.Announce()))

	require.Equal(t, providertest.Announce().Short, text)
}
