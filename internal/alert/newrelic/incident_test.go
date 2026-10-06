package newrelic

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
	"github.com/abahmed/kwatch/internal/delivery/transport"
)

func newTestNewRelic(t *testing.T) (*NewRelic, *providertest.Recorder) {
	t.Helper()
	rec := providertest.NewRecorder(t)
	c := NewNewRelic(map[string]interface{}{
		"apiKey": "key", "accountId": "1",
	}, "dev", rec.Dependencies())
	require.NotNil(t, c)
	c.url = rec.URL() + "/v1/accounts/1/events"
	return c, rec
}

// lastEvent decodes the single event of the last request's array body.
func lastEvent(
	t *testing.T, rec *providertest.Recorder,
) map[string]interface{} {
	t.Helper()
	var events []map[string]interface{}
	require.NoError(t, json.Unmarshal(rec.Last(t).Body, &events))
	require.Len(t, events, 1)
	return events[0]
}

func TestNewRelicSendIncidentLifecycle(t *testing.T) {
	c, rec := newTestNewRelic(t)
	wantState := map[string]string{
		"announce": "firing", "update": "firing", "resolve": "resolved",
	}
	for _, tc := range providertest.Lifecycle() {
		t.Run(tc.Name, func(t *testing.T) {
			require.NoError(t, c.SendIncident(context.Background(),
				tc.Message))
			assert.Equal(t, "key", rec.Last(t).Header.Get("Api-Key"))
			ev := lastEvent(t, rec)
			assert.Equal(t, "KwatchAlert", ev["eventType"])
			assert.Equal(t, "dev", ev["cluster"])
			assert.Equal(t, tc.Message.AlertKey("dev"), ev["incidentKey"])
			assert.Equal(t, wantState[tc.Name], ev["state"])
			assert.Equal(t, tc.Message.Short, ev["title"])
			providertest.AssertOneLeadingEmoji(t, ev["title"].(string))
			assert.Contains(t, ev["message"], tc.Message.Note)
			assert.Equal(t, "shop", ev["namespace"])
			assert.Equal(t, "OOMKilled", ev["reason"])
		})
	}
}

func TestNewRelicTruncatesAttributes(t *testing.T) {
	c, rec := newTestNewRelic(t)
	m := providertest.Announce()
	m.Short = "🔴 " + strings.Repeat("x", 400)
	m.Note = "🔴 " + strings.Repeat("y", 9000)
	require.NoError(t, c.SendIncident(context.Background(), m))
	ev := lastEvent(t, rec)
	assert.LessOrEqual(t, len(ev["title"].(string)), maxTitleBytes)
	assert.LessOrEqual(t, len(ev["message"].(string)), maxMessageBytes)
}

func TestNewRelicSkipsInformationalMessages(t *testing.T) {
	c, rec := newTestNewRelic(t)
	providertest.AssertNoticesSkipped(t, c, rec)
}

func TestNewRelicClassifiesErrors(t *testing.T) {
	c, rec := newTestNewRelic(t)
	rec.Reply = func(w http.ResponseWriter, _ providertest.Request) {
		w.WriteHeader(http.StatusForbidden)
	}
	err := c.SendIncident(context.Background(), providertest.Announce())
	assert.True(t, transport.IsPermanent(err))
	c.url = "h ttp://bad"
	assert.Error(t, c.SendIncident(
		context.Background(), providertest.Announce()))
}
