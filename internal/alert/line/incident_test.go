package line

import (
	"context"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func recorderLine(t *testing.T) (*Line, *providertest.Recorder) {
	t.Helper()
	rec := providertest.NewRecorder(t)
	c := NewLine(map[string]interface{}{"token": "secret"}, "dev",
		rec.Dependencies())
	require.NotNil(t, c)
	c.url = rec.URL() + "/api/notify"
	return c, rec
}

func lineText(t *testing.T, rec *providertest.Recorder) string {
	t.Helper()
	req := rec.Last(t)
	assert.Equal(t, "Bearer secret", req.Header.Get("Authorization"))
	form, err := url.ParseQuery(string(req.Body))
	require.NoError(t, err)
	return form.Get("message")
}

func TestLineSendIncidentRendersNote(t *testing.T) {
	c, rec := recorderLine(t)
	for _, tc := range providertest.Lifecycle() {
		t.Run(tc.Name, func(t *testing.T) {
			m := tc.Message
			require.NoError(t, c.SendIncident(context.Background(), m))
			text := lineText(t, rec)
			assert.True(t, strings.HasPrefix(text, m.Note))
			providertest.AssertOneLeadingEmoji(t, text)
			if len(m.Output) > 0 {
				assert.True(t,
					strings.HasSuffix(text, "\n\n> panic: out of memory"))
			}
		})
	}
}

func TestLineSendIncidentTruncatesMessage(t *testing.T) {
	c, rec := recorderLine(t)
	m := providertest.Announce()
	m.Note = "🔴 " + strings.Repeat("x", lineTextLimit)
	require.NoError(t, c.SendIncident(context.Background(), m))
	text := lineText(t, rec)
	assert.LessOrEqual(t, len(text), lineTextLimit)
	assert.True(t, strings.HasSuffix(text, "…"))
}
