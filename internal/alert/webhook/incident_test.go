package webhook

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// incidentFields is the documented webhook schema. Optional fields are
// omitted when empty; the fixture fills every one except timeline, steps
// and confidence.
var incidentFields = []string{
	"cluster", "key", "alertKey", "revision", "status", "resolved",
	"marker", "short", "note", "title", "lines", "route",
}

func TestWebhookSendIncidentLifecycle(t *testing.T) {
	rec := providertest.NewRecorder(t)
	c := NewWebhook(map[string]interface{}{
		"url": rec.URL(),
		"headers": []interface{}{
			map[string]string{"name": "X-Team", "value": "ops"},
		},
		"basicAuth": map[string]string{"username": "u", "password": "p"},
	}, "dev", rec.Dependencies())
	require.NotNil(t, c)

	for _, tc := range providertest.Lifecycle() {
		t.Run(tc.Name, func(t *testing.T) {
			m := tc.Message
			require.NoError(t, c.SendIncident(context.Background(), m))
			last := rec.Last(t)
			got := last.JSON(t)
			for _, field := range incidentFields {
				assert.Contains(t, got, field)
			}
			assert.Equal(t, "dev", got["cluster"])
			assert.Equal(t, providertest.Key, got["key"])
			assert.Equal(t, "kwatch-dev-"+providertest.Key, got["alertKey"])
			assert.EqualValues(t, m.Revision, got["revision"])
			assert.Equal(t, m.Status.String(), got["status"])
			assert.Equal(t, m.Resolved(), got["resolved"])
			assert.Equal(t, m.Marker, got["marker"])
			assert.Equal(t, m.Short, got["short"])
			assert.Equal(t, m.Note, got["note"])
			assert.Equal(t, m.Title, got["title"])
			assert.Equal(t, len(m.Output) > 0, got["output"] != nil)
			providertest.AssertOneLeadingEmoji(t, got["note"].(string))
			assert.Equal(t, "ops", last.Header.Get("X-Team"))
			user, pass, ok := (&http.Request{Header: last.Header}).
				BasicAuth()
			assert.True(t, ok && user == "u" && pass == "p")
		})
	}
	assert.Equal(t, false, rec.Requests()[0].JSON(t)["resolved"])
}

func TestWebhookSendIncidentReportsHTTPFailure(t *testing.T) {
	rec := providertest.NewRecorder(t)
	rec.Reply = func(w http.ResponseWriter, _ providertest.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}
	c := NewWebhook(map[string]interface{}{"url": rec.URL()}, "dev",
		rec.Dependencies())
	require.NotNil(t, c)
	assert.Error(t, c.SendIncident(context.Background(),
		providertest.Announce()))
}

func TestWebhookPayloadCarriesDeliveryFlags(t *testing.T) {
	rec := providertest.NewRecorder(t)
	c := NewWebhook(map[string]interface{}{"url": rec.URL()},
		"dev", rec.Dependencies())
	require.NotNil(t, c)
	m := providertest.Resolve()
	m.PagingOnly = true
	m.SkipPaging = true
	m.Carrier = "digest"
	require.NoError(t, c.SendIncident(context.Background(), m))

	got := rec.Last(t).JSON(t)
	assert.Equal(t, true, got["pagingOnly"])
	assert.Equal(t, true, got["skipPaging"])
	assert.Equal(t, "digest", got["carrier"])
	assert.True(t, c.ReceivesPagingOnly())
}
