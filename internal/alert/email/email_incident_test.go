package email

import (
	"bytes"
	"context"
	"io"
	"mime"
	"mime/quotedprintable"
	"net/mail"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gomail "gopkg.in/mail.v2"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
	"github.com/abahmed/kwatch/internal/notification"
)

// sentMail is one captured message, decoded as a mail reader would.
type sentMail struct {
	header mail.Header
	body   string
}

func newCapturingEmail(t *testing.T) (*Email, *[]sentMail) {
	t.Helper()
	c := NewEmail(map[string]interface{}{
		"from": "kwatch@test.com", "to": "a@test.com,b@test.com",
		"host": "127.0.0.1", "port": "587",
	}, "dev")
	require.NotNil(t, c)
	var sent []sentMail
	c.send = func(messages ...*gomail.Message) error {
		for _, m := range messages {
			var raw bytes.Buffer
			_, err := m.WriteTo(&raw)
			require.NoError(t, err)
			parsed, err := mail.ReadMessage(&raw)
			require.NoError(t, err)
			body, err := io.ReadAll(parsed.Body)
			require.NoError(t, err)
			sent = append(sent, sentMail{
				header: parsed.Header, body: decodeQP(t, body),
			})
		}
		return nil
	}
	return c, &sent
}

func decodeQP(t *testing.T, body []byte) string {
	t.Helper()
	decoded, err := io.ReadAll(
		quotedprintable.NewReader(bytes.NewReader(body)))
	require.NoError(t, err)
	return strings.ReplaceAll(string(decoded), "\r\n", "\n")
}

func decodedSubject(t *testing.T, h mail.Header) string {
	t.Helper()
	subject, err := new(mime.WordDecoder).DecodeHeader(h.Get("Subject"))
	require.NoError(t, err)
	return subject
}

func TestEmailSendIncidentLifecycle(t *testing.T) {
	c, sent := newCapturingEmail(t)
	for _, tc := range providertest.Lifecycle() {
		t.Run(tc.Name, func(t *testing.T) {
			require.NoError(t, c.SendIncident(context.Background(),
				tc.Message))
			got := (*sent)[len(*sent)-1]
			subject := decodedSubject(t, got.header)
			assert.Equal(t, tc.Message.Short, subject)
			providertest.AssertOneLeadingEmoji(t, subject)
			assert.True(t, strings.HasPrefix(got.body, tc.Message.Note))
			assert.Equal(t, "kwatch-dev-"+providertest.Key,
				got.header.Get("X-Kwatch-Alert-Key"))
			assert.Equal(t, "dev", got.header.Get("X-Kwatch-Cluster"))
			if len(tc.Message.Output) > 0 {
				assert.Contains(t, got.body, "\n\n> panic: out of memory")
			} else {
				assert.NotContains(t, got.body, "> ")
			}
		})
	}
}

func TestEmailSubjectCannotInjectHeaders(t *testing.T) {
	c, sent := newCapturingEmail(t)
	m := providertest.Hostile()
	m.Short = "🔴 api down\r\nBcc: attacker@evil.test"
	require.NoError(t, c.SendIncident(context.Background(), m))
	got := (*sent)[0]
	assert.Empty(t, got.header.Get("Bcc"))
	assert.Equal(t, "🔴 api down Bcc: attacker@evil.test",
		decodedSubject(t, got.header))
	assert.Contains(t, got.body, "<script>")
}

func TestEmailSendMessageIsNotice(t *testing.T) {
	c, sent := newCapturingEmail(t)
	require.NoError(t, c.SendMessage(context.Background(),
		"kwatch started\nversion 1"))
	got := (*sent)[0]
	assert.Equal(t, "kwatch started", decodedSubject(t, got.header))
	assert.Equal(t, "kwatch started\nversion 1", got.body)
	assert.Equal(t, notification.Notice("x").AlertKey("dev"),
		got.header.Get("X-Kwatch-Alert-Key"))
}
