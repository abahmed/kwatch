package sns

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
	"github.com/abahmed/kwatch/internal/delivery/transport"
)

const topic = "arn:aws:sns:us-east-1:123456789012:kwatch"

func newTestSns(
	t *testing.T, extra map[string]interface{},
) (*Sns, *providertest.Recorder) {
	t.Helper()
	rec := providertest.NewRecorder(t)
	config := map[string]interface{}{
		"accessKeyId": "AKIA123", "secretAccessKey": "s", "topicArn": topic,
	}
	for name, value := range extra {
		config[name] = value
	}
	c := NewSns(config, "dev", rec.Dependencies())
	require.NotNil(t, c)
	c.url = rec.URL() + "/"
	return c, rec
}

func lastForm(t *testing.T, rec *providertest.Recorder) url.Values {
	t.Helper()
	form, err := url.ParseQuery(string(rec.Last(t).Body))
	require.NoError(t, err)
	return form
}

func TestSnsSendIncidentLifecycle(t *testing.T) {
	c, rec := newTestSns(t, nil)
	for _, tc := range providertest.Lifecycle() {
		t.Run(tc.Name, func(t *testing.T) {
			require.NoError(t, c.SendIncident(context.Background(),
				tc.Message))
			auth := rec.Last(t).Header.Get("Authorization")
			assert.Contains(t, auth, "AWS4-HMAC-SHA256")
			form := lastForm(t, rec)
			assert.Equal(t, "Publish", form.Get("Action"))
			assert.Equal(t, topic, form.Get("TopicArn"))
			providertest.AssertOneLeadingEmoji(t, form.Get("Message"))
			assert.Contains(t, form.Get("Message"), tc.Message.Note)
			short := strings.TrimPrefix(tc.Message.Short, "🔴 ")
			short = strings.TrimPrefix(short, "✅ ")
			assert.Equal(t, short, form.Get("Subject"))
			assert.Equal(t, "kwatch.key",
				form.Get("MessageAttributes.entry.1.Name"))
			assert.Equal(t, tc.Message.AlertKey("dev"),
				form.Get("MessageAttributes.entry.1.Value.StringValue"))
			assert.Equal(t, tc.Message.Status.String(),
				form.Get("MessageAttributes.entry.2.Value.StringValue"))
		})
	}
	assert.Equal(t, "resolved", lastForm(t, rec).Get(
		"MessageAttributes.entry.2.Value.StringValue"))
}

func TestSnsSubjectIsASCIIAndBounded(t *testing.T) {
	assert.Equal(t, "pay is down", asciiSubject("🔴 pay is down\n"))
	long := asciiSubject("🔴 " + strings.Repeat("x", 300))
	assert.Len(t, long, maxSubjectBytes)

	c, rec := newTestSns(t, map[string]interface{}{"subject": "kwatch"})
	require.NoError(t, c.SendIncident(context.Background(),
		providertest.Announce()))
	assert.Equal(t, "kwatch", lastForm(t, rec).Get("Subject"))
}

func TestSnsSendMessageIsPlain(t *testing.T) {
	c, rec := newTestSns(t, map[string]interface{}{
		"targetArn": "arn:endpoint",
	})
	require.NoError(t, c.SendMessage(context.Background(), "hello"))
	form := lastForm(t, rec)
	assert.Equal(t, "hello", form.Get("Message"))
	assert.Equal(t, "arn:endpoint", form.Get("TargetArn"))
	assert.Empty(t, form.Get("TopicArn"))
	assert.Empty(t, form.Get("MessageAttributes.entry.1.Name"))
}

func TestSnsClassifiesErrors(t *testing.T) {
	c, rec := newTestSns(t, nil)
	rec.Reply = func(w http.ResponseWriter, _ providertest.Request) {
		w.WriteHeader(http.StatusForbidden)
	}
	err := c.SendIncident(context.Background(), providertest.Announce())
	assert.True(t, transport.IsPermanent(err))
	c.url = "h ttp://bad"
	assert.Error(t, c.SendMessage(context.Background(), "x"))
}
