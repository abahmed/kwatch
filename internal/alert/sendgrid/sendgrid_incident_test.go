package sendgrid

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func newRecordedSendgrid(
	t *testing.T,
) (*Sendgrid, *providertest.Recorder) {
	t.Helper()
	rec := providertest.NewRecorder(t)
	c := NewSendgrid(map[string]interface{}{
		"apiKey": "key", "from": "kwatch@example.com",
		"to": "ops@example.com", "subject": "custom subject",
	}, "dev", rec.Dependencies())
	require.NotNil(t, c)
	c.url = rec.URL()
	return c, rec
}

func sentPayload(
	t *testing.T, rec *providertest.Recorder,
) sendgridPayload {
	t.Helper()
	var payload sendgridPayload
	require.NoError(t, json.Unmarshal(rec.Last(t).Body, &payload))
	return payload
}

func TestSendgridSendIncidentLifecycle(t *testing.T) {
	c, rec := newRecordedSendgrid(t)
	for _, tc := range providertest.Lifecycle() {
		t.Run(tc.Name, func(t *testing.T) {
			require.NoError(t, c.SendIncident(context.Background(),
				tc.Message))
			payload := sentPayload(t, rec)
			assert.Equal(t, tc.Message.Short, payload.Subject)
			providertest.AssertOneLeadingEmoji(t, payload.Subject)
			require.Len(t, payload.Content, 1)
			assert.Equal(t, "text/plain", payload.Content[0].Type)
			body := payload.Content[0].Value
			assert.True(t, strings.HasPrefix(body, tc.Message.Note))
			if len(tc.Message.Output) > 0 {
				assert.Contains(t, body, "\n\n> panic: out of memory")
			} else {
				assert.Equal(t, tc.Message.Note, body)
			}
			assert.Equal(t, "Bearer key",
				rec.Last(t).Header.Get("Authorization"))
		})
	}
}

func TestSendgridSubjectFoldsLineBreaks(t *testing.T) {
	c, rec := newRecordedSendgrid(t)
	m := providertest.Hostile()
	m.Short = "🔴 api\r\nBcc: x@evil.test"
	require.NoError(t, c.SendIncident(context.Background(), m))
	assert.Equal(t, "🔴 api Bcc: x@evil.test", sentPayload(t, rec).Subject)
}

func TestSendgridSendMessageUsesConfiguredSubject(t *testing.T) {
	c, rec := newRecordedSendgrid(t)
	require.NoError(t, c.SendMessage(context.Background(), "hello"))
	payload := sentPayload(t, rec)
	assert.Equal(t, "custom subject", payload.Subject)
	assert.Equal(t, "hello", payload.Content[0].Value)
}
