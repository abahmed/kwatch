package telegram

import (
	"context"
	"encoding/json"
	"html"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func recorderTelegram(t *testing.T) (*Telegram, *providertest.Recorder) {
	t.Helper()
	rec := providertest.NewRecorder(t)
	c := NewTelegram(
		map[string]interface{}{"token": "tok", "chatId": "chat"},
		testAppConfig(), rec.Dependencies(),
	)
	require.NotNil(t, c)
	c.url = rec.URL() + "/bot%s/sendMessage"
	return c, rec
}

func TestTelegramPlainMessageHasNoParseMode(t *testing.T) {
	c, rec := recorderTelegram(t)
	msg := "pod my_app crashed: *oops* [x]"
	require.NoError(t, c.SendMessage(context.Background(), msg))
	body := rec.Last(t).JSON(t)
	assert.NotContains(t, body, "parse_mode")
	assert.Equal(t, msg, body["text"])
}

func TestTelegramIncidentEscapesHTML(t *testing.T) {
	c, rec := recorderTelegram(t)
	m := providertest.Announce()
	m.Note = "🔴 my_app hit *bad* `x` [y]"
	m.Output = []string{"panic: `boom`"}
	require.NoError(t, c.SendIncident(context.Background(), m))
	body := rec.Last(t).JSON(t)
	assert.Equal(t, "HTML", body["parse_mode"])
	assert.Equal(t,
		"🔴 my_app hit *bad* `x` [y]\n\n<pre>panic: `boom`</pre>",
		body["text"])
}

func TestTelegramIncidentStaysWithinLimit(t *testing.T) {
	c := &Telegram{chatId: "chat"}
	m := providertest.Announce()
	m.Note = "🔴 " + strings.Repeat("a_", telegramTextLimit)
	var payload telegramPayload
	require.NoError(t, json.Unmarshal([]byte(c.incidentBody(m)), &payload))
	assert.LessOrEqual(t, len(payload.Text), telegramTextLimit)
	providertest.AssertOneLeadingEmoji(t, payload.Text)
}

func TestTelegramIncidentTruncationKeepsCodeFence(t *testing.T) {
	tests := []struct {
		name      string
		noteLen   int
		outputLen int
	}{
		{name: "long output", noteLen: 100, outputLen: 10000},
		{name: "long note", noteLen: 10000, outputLen: 100},
		{name: "both long", noteLen: 10000, outputLen: 10000},
		{name: "both short", noteLen: 100, outputLen: 100},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := providertest.Announce()
			m.Note = "🔴 " + strings.Repeat("n", tc.noteLen)
			m.Output = []string{strings.Repeat("o", tc.outputLen)}

			text := incidentText(m, telegramTextLimit)

			assert.LessOrEqual(t, len(text), telegramTextLimit)
			assert.True(t, strings.HasSuffix(text, "</pre>"),
				"closing fence must survive: %q", text[len(text)-20:])
			assert.Equal(t, 1, strings.Count(text, "<pre>"))
			assert.Contains(t, text, "\n\n<pre>oo")
		})
	}
}

func TestTelegramSendIncidentRendersNote(t *testing.T) {
	c, rec := recorderTelegram(t)
	for _, tc := range providertest.Lifecycle() {
		t.Run(tc.Name, func(t *testing.T) {
			m := tc.Message
			require.NoError(t, c.SendIncident(context.Background(), m))
			req := rec.Last(t)
			assert.Equal(t, "/bottok/sendMessage", req.Path)
			body := req.JSON(t)
			assert.Equal(t, "chat", body["chat_id"])
			text := body["text"].(string)
			assert.True(t, strings.HasPrefix(text, html.EscapeString(m.Note)))
			providertest.AssertOneLeadingEmoji(t, text)
		})
	}
}
