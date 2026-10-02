package feishu

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func recorderFeiShu(t *testing.T) (*FeiShu, *providertest.Recorder) {
	t.Helper()
	rec := providertest.NewRecorder(t)
	c := NewFeiShu(map[string]interface{}{
		"webhook": rec.URL() + "/hook", "title": "kwatch",
	}, "dev", rec.Dependencies())
	require.NotNil(t, c)
	return c, rec
}

func cardText(t *testing.T, rec *providertest.Recorder) string {
	t.Helper()
	body := rec.Last(t).JSON(t)
	assert.Equal(t, "interactive", body["msg_type"])
	card := body["card"].(map[string]any)
	element := card["elements"].([]any)[0].(map[string]any)
	assert.Equal(t, "markdown", element["tag"])
	return element["content"].(string)
}

func TestFeiShuSendIncidentRendersNote(t *testing.T) {
	c, rec := recorderFeiShu(t)
	for _, tc := range providertest.Lifecycle() {
		t.Run(tc.Name, func(t *testing.T) {
			m := tc.Message
			require.NoError(t, c.SendIncident(context.Background(), m))
			text := cardText(t, rec)
			assert.True(t, strings.HasPrefix(text, m.Note))
			providertest.AssertOneLeadingEmoji(t, text)
		})
	}
}

func TestFeiShuSendIncidentTruncatesCard(t *testing.T) {
	c, rec := recorderFeiShu(t)
	m := providertest.Announce()
	m.Note = "🔴 " + strings.Repeat("x", feiShuTextLimit)
	require.NoError(t, c.SendIncident(context.Background(), m))
	text := cardText(t, rec)
	assert.LessOrEqual(t, len(text), feiShuTextLimit)
	assert.True(t, strings.HasSuffix(text, "…"))
}

func TestFeiShuSendMessageUsesConfiguredTitle(t *testing.T) {
	c, rec := recorderFeiShu(t)
	require.NoError(t, c.SendMessage(context.Background(), "kwatch started"))
	body := rec.Last(t).JSON(t)
	card := body["card"].(map[string]any)
	title := card["header"].(map[string]any)["title"].(map[string]any)
	assert.Equal(t, "kwatch", title["content"])
	assert.Equal(t, "kwatch started", cardText(t, rec))
}
