package ses

import (
	"context"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func newRecordedSes(t *testing.T) (*Ses, *providertest.Recorder) {
	t.Helper()
	rec := providertest.NewRecorder(t)
	c := NewSes(map[string]interface{}{
		"accessKeyId": "AKIA123", "secretAccessKey": "test",
		"from": "kwatch@example.com", "to": "ops@example.com",
		"subject": "custom subject",
	}, "dev", rec.Dependencies())
	require.NotNil(t, c)
	c.url = rec.URL()
	return c, rec
}

func sentForm(t *testing.T, rec *providertest.Recorder) url.Values {
	t.Helper()
	form, err := url.ParseQuery(string(rec.Last(t).Body))
	require.NoError(t, err)
	return form
}

func TestSesSendIncidentLifecycle(t *testing.T) {
	c, rec := newRecordedSes(t)
	for _, tc := range providertest.Lifecycle() {
		t.Run(tc.Name, func(t *testing.T) {
			require.NoError(t, c.SendIncident(context.Background(),
				tc.Message))
			form := sentForm(t, rec)
			subject := form.Get("Message.Subject.Data")
			assert.Equal(t, tc.Message.Short, subject)
			providertest.AssertOneLeadingEmoji(t, subject)
			body := form.Get("Message.Body.Text.Data")
			assert.True(t, strings.HasPrefix(body, tc.Message.Note))
			if len(tc.Message.Output) > 0 {
				assert.Contains(t, body, "\n\n> panic: out of memory")
			} else {
				assert.Equal(t, tc.Message.Note, body)
			}
			assert.Contains(t, rec.Last(t).Header.Get("Authorization"),
				"AWS4-HMAC-SHA256")
		})
	}
}

func TestSesSubjectFoldsLineBreaks(t *testing.T) {
	c, rec := newRecordedSes(t)
	m := providertest.Hostile()
	m.Short = "🔴 api\r\nBcc: x@evil.test"
	require.NoError(t, c.SendIncident(context.Background(), m))
	assert.Equal(t, "🔴 api Bcc: x@evil.test",
		sentForm(t, rec).Get("Message.Subject.Data"))
}

func TestSesSendMessageUsesConfiguredSubject(t *testing.T) {
	c, rec := newRecordedSes(t)
	require.NoError(t, c.SendMessage(context.Background(), "hello"))
	form := sentForm(t, rec)
	assert.Equal(t, "custom subject", form.Get("Message.Subject.Data"))
	assert.Equal(t, "hello", form.Get("Message.Body.Text.Data"))
}
