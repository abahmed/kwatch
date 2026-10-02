package zenduty

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
	"github.com/abahmed/kwatch/internal/delivery/transport"
)

func newTestZenduty(
	t *testing.T, extra map[string]interface{},
) (*Zenduty, *providertest.Recorder) {
	t.Helper()
	rec := providertest.NewRecorder(t)
	config := map[string]interface{}{"integrationKey": "key"}
	for name, value := range extra {
		config[name] = value
	}
	c := NewZenduty(config, "dev", rec.Dependencies())
	require.NotNil(t, c)
	c.url = rec.URL()
	return c, rec
}

func TestZendutySendIncidentLifecycle(t *testing.T) {
	c, rec := newTestZenduty(t, nil)
	wantType := map[string]string{
		"announce": "critical", "update": "critical", "resolve": "resolved",
	}
	for _, tc := range providertest.Lifecycle() {
		t.Run(tc.Name, func(t *testing.T) {
			require.NoError(t, c.SendIncident(context.Background(),
				tc.Message))
			req := rec.Last(t)
			body := req.JSON(t)
			assert.Equal(t, "/key/", req.Path)
			assert.Equal(t, tc.Message.AlertKey("dev"), body["entity_id"])
			assert.Equal(t, wantType[tc.Name], body["alert_type"])
			assert.Equal(t, tc.Message.Short, body["message"])
			providertest.AssertOneLeadingEmoji(t,
				body["message"].(string))
			assert.Contains(t, body["summary"], tc.Message.Note)
			assert.Contains(t, body["summary"], "Cluster: dev")
		})
	}
}

func TestZendutyConfiguredAlertTypeWinsUntilResolve(t *testing.T) {
	c, rec := newTestZenduty(t, map[string]interface{}{"alertType": "info"})
	require.NoError(t, c.SendIncident(context.Background(),
		providertest.Announce()))
	assert.Equal(t, "info", rec.Last(t).JSON(t)["alert_type"])
	require.NoError(t, c.SendIncident(context.Background(),
		providertest.Resolve()))
	assert.Equal(t, "resolved", rec.Last(t).JSON(t)["alert_type"])
}

func TestZendutyTruncatesMessage(t *testing.T) {
	c, rec := newTestZenduty(t, nil)
	m := providertest.Announce()
	m.Short = "🔴 " + strings.Repeat("x", 300)
	require.NoError(t, c.SendIncident(context.Background(), m))
	msg := rec.Last(t).JSON(t)["message"].(string)
	assert.LessOrEqual(t, len(msg), maxMessageBytes)
	assert.True(t, strings.HasSuffix(msg, "…"))
}

func TestZendutyClassifiesErrors(t *testing.T) {
	c, rec := newTestZenduty(t, nil)
	rec.Reply = func(w http.ResponseWriter, _ providertest.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}
	err := c.SendIncident(context.Background(), providertest.Announce())
	assert.True(t, transport.IsPermanent(err))

	rec.Reply = func(w http.ResponseWriter, _ providertest.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}
	err = c.SendIncident(context.Background(), providertest.Announce())
	require.Error(t, err)
	assert.False(t, transport.IsPermanent(err))
}

func TestZendutySkipsNotices(t *testing.T) {
	c, rec := newTestZenduty(t, nil)
	providertest.AssertNoticesSkipped(t, c, rec)
}
