package wecom

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func recorderWecom(t *testing.T) (*Wecom, *providertest.Recorder) {
	t.Helper()
	rec := providertest.NewRecorder(t)
	c := NewWecom(map[string]interface{}{"webhook": rec.URL()}, "dev",
		rec.Dependencies())
	require.NotNil(t, c)
	return c, rec
}

func markdownContent(t *testing.T, rec *providertest.Recorder) string {
	t.Helper()
	body := rec.Last(t).JSON(t)
	assert.Equal(t, "markdown", body["msgtype"])
	return body["markdown"].(map[string]any)["content"].(string)
}

func TestWecomSendIncidentRendersNote(t *testing.T) {
	c, rec := recorderWecom(t)
	for _, tc := range providertest.Lifecycle() {
		t.Run(tc.Name, func(t *testing.T) {
			m := tc.Message
			require.NoError(t, c.SendIncident(context.Background(), m))
			content := markdownContent(t, rec)
			assert.True(t, strings.HasPrefix(content, m.Note))
			providertest.AssertOneLeadingEmoji(t, content)
		})
	}
}

func TestWecomSendIncidentTruncatesContent(t *testing.T) {
	c, rec := recorderWecom(t)
	m := providertest.Announce()
	m.Note = "🔴 " + strings.Repeat("x", wecomTextLimit)
	require.NoError(t, c.SendIncident(context.Background(), m))
	content := markdownContent(t, rec)
	assert.LessOrEqual(t, len(content), wecomTextLimit)
	assert.True(t, strings.HasSuffix(content, "…"))
}
