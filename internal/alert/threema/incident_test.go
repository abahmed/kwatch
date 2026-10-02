package threema

import (
	"context"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

func recorderThreema(t *testing.T) (*Threema, *providertest.Recorder) {
	t.Helper()
	rec := providertest.NewRecorder(t)
	c := NewThreema(map[string]interface{}{
		"gatewayId": "*KWATCH1", "secret": "secret", "to": "ABCD1234",
	}, "dev", rec.Dependencies())
	require.NotNil(t, c)
	c.url = rec.URL() + "/send_simple"
	return c, rec
}

func threemaText(t *testing.T, rec *providertest.Recorder) string {
	t.Helper()
	form, err := url.ParseQuery(string(rec.Last(t).Body))
	require.NoError(t, err)
	assert.Equal(t, "ABCD1234", form.Get("to"))
	return form.Get("text")
}

func TestThreemaSendIncidentRendersNote(t *testing.T) {
	c, rec := recorderThreema(t)
	for _, tc := range providertest.Lifecycle() {
		t.Run(tc.Name, func(t *testing.T) {
			m := tc.Message
			require.NoError(t, c.SendIncident(context.Background(), m))
			text := threemaText(t, rec)
			assert.True(t, strings.HasPrefix(text, m.Note))
			providertest.AssertOneLeadingEmoji(t, text)
		})
	}
}

func TestThreemaSendIncidentTruncatesText(t *testing.T) {
	c, rec := recorderThreema(t)
	m := providertest.Announce()
	m.Note = "🔴 " + strings.Repeat("x", threemaTextLimit)
	require.NoError(t, c.SendIncident(context.Background(), m))
	text := threemaText(t, rec)
	assert.LessOrEqual(t, len(text), threemaTextLimit)
	assert.True(t, strings.HasSuffix(text, "…"))
}
