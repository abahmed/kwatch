package resend

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func newRecordedResend(t *testing.T) (*Resend, *providertest.Recorder) {
	t.Helper()
	rec := providertest.NewRecorder(t)
	c := NewResend(map[string]interface{}{
		"apiKey": "key", "from": "kwatch@example.com",
		"to": "ops@example.com", "subject": "custom subject",
	}, "dev", rec.Dependencies())
	require.NotNil(t, c)
	c.url = rec.URL()
	return c, rec
}

func TestResendSendIncidentLifecycle(t *testing.T) {
	c, rec := newRecordedResend(t)
	for _, tc := range providertest.Lifecycle() {
		t.Run(tc.Name, func(t *testing.T) {
			require.NoError(t, c.SendIncident(context.Background(),
				tc.Message))
			payload := rec.Last(t).JSON(t)
			subject, _ := payload["subject"].(string)
			assert.Equal(t, tc.Message.Short, subject)
			providertest.AssertOneLeadingEmoji(t, subject)
			body, _ := payload["text"].(string)
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

func TestResendSubjectFoldsLineBreaks(t *testing.T) {
	c, rec := newRecordedResend(t)
	m := providertest.Hostile()
	m.Short = "🔴 api\r\nBcc: x@evil.test"
	require.NoError(t, c.SendIncident(context.Background(), m))
	assert.Equal(t, "🔴 api Bcc: x@evil.test",
		rec.Last(t).JSON(t)["subject"])
}

func TestResendSendMessageUsesConfiguredSubject(t *testing.T) {
	c, rec := newRecordedResend(t)
	require.NoError(t, c.SendMessage(context.Background(), "hello"))
	payload := rec.Last(t).JSON(t)
	assert.Equal(t, "custom subject", payload["subject"])
	assert.Equal(t, "hello", payload["text"])
}
