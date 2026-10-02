package slack

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	slackClient "github.com/slack-go/slack"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

const okPost = `{"ok":true,"channel":"C42","ts":"111.1"}`

// namedChannelSlack is a token Slack configured with a channel name, so
// Slack answers posts with a channel ID that differs from the setting.
func namedChannelSlack(
	t *testing.T, reply func(path string) string,
) (*Slack, *providertest.Recorder) {
	t.Helper()
	rec := providertest.NewRecorder(t)
	rec.Reply = func(w http.ResponseWriter, r providertest.Request) {
		_, _ = w.Write([]byte(reply(r.Path)))
	}
	s := NewSlack(map[string]interface{}{
		"token": "xoxb-test", "channel": "#alerts",
	}, "dev", rec.Dependencies())
	require.NotNil(t, s)
	s.apiClient = slackClient.New("xoxb-test",
		slackClient.OptionHTTPClient(rec.Server.Client()),
		slackClient.OptionAPIURL(rec.URL()+"/"))
	return s, rec
}

func formOf(t *testing.T, req providertest.Request) url.Values {
	t.Helper()
	form, err := url.ParseQuery(string(req.Body))
	require.NoError(t, err)
	return form
}

func TestSlackTokenPostsCarryTextFallback(t *testing.T) {
	s, rec := namedChannelSlack(t, func(string) string { return okPost })
	ctx := context.Background()
	require.NoError(t, s.SendIncident(ctx, providertest.Announce()))
	require.NoError(t, s.SendIncident(ctx, providertest.Update()))

	requests := rec.Requests()
	require.Len(t, requests, 4)
	for i, req := range requests[:3] {
		want := providertest.Announce().Short
		if i == 2 {
			want = providertest.Update().Short
		}
		require.Equal(t, want, formOf(t, req).Get("text"), i)
	}
	update := formOf(t, requests[3])
	require.Equal(t, "/chat.update", requests[3].Path)
	require.Equal(t, providertest.Update().Short, update.Get("text"))
}

func TestSlackTextFallbackNeutralizesMentions(t *testing.T) {
	rec := providertest.NewRecorder(t)
	s := NewSlack(map[string]interface{}{"webhook": rec.URL()}, "dev",
		rec.Dependencies())
	m := providertest.Announce()
	m.Short = "🔴 @channel pay <b>&</b>"

	require.NoError(t, s.SendIncident(context.Background(), m))

	var msg slackClient.WebhookMessage
	require.NoError(t, json.Unmarshal(rec.Last(t).Body, &msg))
	require.NotContains(t, msg.Text, "@channel")
	require.Contains(t, msg.Text, "&lt;b&gt;&amp;&lt;/b&gt;")
}

func TestSlackRootEditUsesPostedChannelID(t *testing.T) {
	s, rec := namedChannelSlack(t, func(string) string { return okPost })
	require.Equal(t, "#alerts", s.updateChannel(), "before any post")
	ctx := context.Background()
	require.NoError(t, s.SendIncident(ctx, providertest.Announce()))
	require.NoError(t, s.SendIncident(ctx, providertest.Update()))

	last := rec.Last(t)
	require.Equal(t, "/chat.update", last.Path)
	require.Equal(t, "C42", formOf(t, last).Get("channel"))
	require.Equal(t, "111.1", s.SnapshotThreads()[providertest.Key])
}

// failingUpdate answers chat.update with an error. Once the update has
// failed, posts fail too when failPostsAfter is set.
func failingUpdate(failPostsAfter bool) func(path string) string {
	var updated atomic.Bool
	return func(path string) string {
		if strings.HasSuffix(path, "chat.update") {
			updated.Store(true)
			return `{"ok":false,"error":"cant_update_message"}`
		}
		if failPostsAfter && updated.Load() {
			return `{"ok":false,"error":"internal_error"}`
		}
		return okPost
	}
}

func TestSlackFailedRootEditPostsStatusInThread(t *testing.T) {
	s, rec := namedChannelSlack(t, failingUpdate(false))
	ctx := context.Background()
	require.NoError(t, s.SendIncident(ctx, providertest.Announce()))
	rec.Reset()

	require.NoError(t, s.SendIncident(ctx, providertest.Update()))

	requests := rec.Requests()
	require.Len(t, requests, 3, "note, failed edit, status post")
	require.Equal(t, "/chat.update", requests[1].Path)
	status := formOf(t, requests[2])
	require.Equal(t, "/chat.postMessage", requests[2].Path)
	require.Equal(t, "111.1", status.Get("thread_ts"))
	require.Equal(t, providertest.Update().Short,
		decodeCall(t, requests[2]).text)
}

func TestSlackFailedStatusPostDoesNotFailDelivery(t *testing.T) {
	s, rec := namedChannelSlack(t, failingUpdate(true))
	ctx := context.Background()
	require.NoError(t, s.SendIncident(ctx, providertest.Announce()))
	rec.Reset()

	require.NoError(t, s.SendIncident(ctx, providertest.Update()))

	require.Len(t, rec.Requests(), 3, "note, failed edit, failed status")
	require.Equal(t, "111.1", s.SnapshotThreads()[providertest.Key])
}
