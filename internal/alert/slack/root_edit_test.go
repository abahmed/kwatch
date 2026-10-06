package slack

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	slackClient "github.com/slack-go/slack"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/ratelimit"
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

// downgrade is an update that changes the incident's status marker, so the
// root has to be edited.
func downgrade() notification.Message {
	m := providertest.Update()
	m.Status = notification.StatusWarning
	m.Marker = notification.MarkerNotify
	m.Short = swapMarker(m.Short, m.Marker)
	m.Note = swapMarker(m.Note, m.Marker)
	return m
}

func TestSlackTokenPostsCarryTextFallback(t *testing.T) {
	s, rec := namedChannelSlack(t, func(string) string { return okPost })
	ctx := context.Background()
	require.NoError(t, s.SendIncident(ctx, providertest.Announce()))
	require.NoError(t, s.SendIncident(ctx, downgrade()))

	requests := rec.Requests()
	require.Len(t, requests, 3, "root, thread reply, root edit")
	require.Equal(t, providertest.Announce().Short,
		formOf(t, requests[0]).Get("text"))
	require.Equal(t, downgrade().Short, formOf(t, requests[1]).Get("text"))
	edit := formOf(t, requests[2])
	require.Equal(t, "/chat.update", requests[2].Path)
	require.Equal(t,
		swapMarker(providertest.Announce().Short, notification.MarkerNotify),
		edit.Get("text"))
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
	require.NoError(t, s.SendIncident(ctx, downgrade()))

	last := rec.Last(t)
	require.Equal(t, "/chat.update", last.Path)
	require.Equal(t, "C42", formOf(t, last).Get("channel"))
	require.Equal(t, "111.1", threadTS(s, providertest.Key))
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

// A permanent refusal of the root edit is logged, not retried, and no
// second status message is posted: the update is already in the thread.
func TestSlackRefusedRootEditDoesNotPostASecondStatus(t *testing.T) {
	s, rec := namedChannelSlack(t, failingUpdate(false))
	ctx := context.Background()
	require.NoError(t, s.SendIncident(ctx, providertest.Announce()))
	rec.Reset()

	require.NoError(t, s.SendIncident(ctx, downgrade()))

	requests := rec.Requests()
	require.Len(t, requests, 2, "thread reply, refused edit")
	require.Equal(t, "/chat.postMessage", requests[0].Path)
	require.Equal(t, "/chat.update", requests[1].Path)
	require.Equal(t, "111.1", threadTS(s, providertest.Key))
}

// rateLimitedUpdate answers chat.update with 429 until released.
func rateLimitedUpdate(limited *atomic.Bool) func(
	http.ResponseWriter, providertest.Request,
) {
	return func(w http.ResponseWriter, r providertest.Request) {
		if strings.HasSuffix(r.Path, "chat.update") && limited.Load() {
			w.Header().Set("Retry-After", "7")
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(okPost))
	}
}

func TestSlackRateLimitedRootEditIsSurfacedWithoutDoublePosting(t *testing.T) {
	s, rec := namedChannelSlack(t, func(string) string { return okPost })
	var limited atomic.Bool
	rec.Reply = rateLimitedUpdate(&limited)
	ctx := context.Background()
	require.NoError(t, s.SendIncident(ctx, providertest.Announce()))
	rec.Reset()

	limited.Store(true)
	err := s.SendIncident(ctx, downgrade())
	var rl *ratelimit.Error
	require.ErrorAs(t, err, &rl)
	require.Equal(t, 7*time.Second, rl.RetryAfter)
	require.Len(t, rec.Requests(), 2, "thread reply and the limited edit")

	// The retry only repeats the edit: the update is already posted.
	limited.Store(false)
	rec.Reset()
	require.NoError(t, s.SendIncident(ctx, downgrade()))
	requests := rec.Requests()
	require.Len(t, requests, 1)
	require.Equal(t, "/chat.update", requests[0].Path)
}

func TestSlackRootEditKeepsAnnouncementDetails(t *testing.T) {
	s, rec := namedChannelSlack(t, func(string) string { return okPost })
	ctx := context.Background()
	require.NoError(t, s.SendIncident(ctx, providertest.Announce()))
	require.NoError(t, s.SendIncident(ctx, downgrade()))

	edit := rec.Last(t)
	var blocks []struct {
		Text struct {
			Text string `json:"text"`
		} `json:"text"`
	}
	require.NoError(t, json.Unmarshal(
		[]byte(formOf(t, edit).Get("blocks")), &blocks))
	require.Len(t, blocks, 3, "note, output label, output")
	require.Equal(t,
		swapMarker(providertest.Announce().Note, notification.MarkerNotify),
		blocks[0].Text.Text)
	require.NotContains(t, blocks[0].Text.Text, "restarted 5 times")
	require.Contains(t, blocks[2].Text.Text, "panic: out of memory")
}

func TestSlackRootNotEditedWhenStatusMarkerIsUnchanged(t *testing.T) {
	s, rec := namedChannelSlack(t, func(string) string { return okPost })
	ctx := context.Background()
	require.NoError(t, s.SendIncident(ctx, providertest.Announce()))
	require.NoError(t, s.SendIncident(ctx, providertest.Update()))

	for _, req := range rec.Requests() {
		require.NotEqual(t, "/chat.update", req.Path)
	}
}

func TestSlackLongAnnouncementStaysWithinSlackLimits(t *testing.T) {
	m := providertest.Announce()
	m.Note += strings.Repeat(" é", maxSectionTextChars)
	m.Output = []string{strings.Repeat("x", maxSectionTextChars*2)}
	for _, block := range noteBlocks(m).BlockSet {
		text := block.(slackClient.SectionBlock).Text.Text
		require.LessOrEqual(t, len([]rune(text)), maxSectionTextChars)
	}
	s, recorder := threadedSlack(t)

	require.NoError(t, s.SendIncident(context.Background(), m))

	require.Len(t, recorder.posts, 1)
	require.True(t, strings.HasSuffix(firstText(recorder.posts[0].blocks),
		"..."))
}
