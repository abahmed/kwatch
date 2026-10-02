package mailgun

import (
	"context"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func newRecordedMailgun(
	t *testing.T,
) (*Mailgun, *providertest.Recorder) {
	t.Helper()
	rec := providertest.NewRecorder(t)
	c := NewMailgun(map[string]interface{}{
		"apiKey": "key", "domain": "mg.example.com",
		"from": "kwatch@example.com", "to": "ops@example.com",
		"subject": "custom subject", "url": rec.URL(),
	}, "dev", rec.Dependencies())
	require.NotNil(t, c)
	return c, rec
}

func sentForm(t *testing.T, rec *providertest.Recorder) url.Values {
	t.Helper()
	last := rec.Last(t)
	assert.Equal(t, "/mg.example.com/messages", last.Path)
	form, err := url.ParseQuery(string(last.Body))
	require.NoError(t, err)
	return form
}

func TestMailgunSendIncidentLifecycle(t *testing.T) {
	c, rec := newRecordedMailgun(t)
	for _, tc := range providertest.Lifecycle() {
		t.Run(tc.Name, func(t *testing.T) {
			require.NoError(t, c.SendIncident(context.Background(),
				tc.Message))
			form := sentForm(t, rec)
			assert.Equal(t, tc.Message.Short, form.Get("subject"))
			providertest.AssertOneLeadingEmoji(t, form.Get("subject"))
			body := form.Get("text")
			assert.True(t, strings.HasPrefix(body, tc.Message.Note))
			if len(tc.Message.Output) > 0 {
				assert.Contains(t, body, "\n\n> panic: out of memory")
			} else {
				assert.Equal(t, tc.Message.Note, body)
			}
		})
	}
}

func TestMailgunSubjectFoldsLineBreaks(t *testing.T) {
	c, rec := newRecordedMailgun(t)
	m := providertest.Hostile()
	m.Short = "🔴 api\r\nBcc: x@evil.test"
	require.NoError(t, c.SendIncident(context.Background(), m))
	assert.Equal(t, "🔴 api Bcc: x@evil.test",
		sentForm(t, rec).Get("subject"))
}

func TestMailgunSendMessageUsesConfiguredSubject(t *testing.T) {
	c, rec := newRecordedMailgun(t)
	require.NoError(t, c.SendMessage(context.Background(), "hello"))
	form := sentForm(t, rec)
	assert.Equal(t, "custom subject", form.Get("subject"))
	assert.Equal(t, "hello", form.Get("text"))
}
