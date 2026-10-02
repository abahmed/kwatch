package sensugo

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/notification"
)

func newTestSensugo(t *testing.T) (*Sensugo, *providertest.Recorder) {
	t.Helper()
	rec := providertest.NewRecorder(t)
	c := NewSensugo(map[string]interface{}{
		"url": rec.URL(), "apiKey": "key",
	}, "dev", rec.Dependencies())
	require.NotNil(t, c)
	return c, rec
}

func TestSensugoSendIncidentLifecycle(t *testing.T) {
	c, rec := newTestSensugo(t)
	wantStatus := map[string]float64{"announce": 2, "update": 2, "resolve": 0}
	for _, tc := range providertest.Lifecycle() {
		t.Run(tc.Name, func(t *testing.T) {
			require.NoError(t, c.SendIncident(context.Background(),
				tc.Message))
			req := rec.Last(t)
			assert.Equal(t, "Key key", req.Header.Get("Authorization"))
			assert.Equal(t, "/api/core/v2/namespaces/default/events",
				req.Path)
			body := req.JSON(t)
			check := body["check"].(map[string]any)
			meta := check["metadata"].(map[string]any)
			assert.Equal(t, tc.Message.AlertKey("dev"), meta["name"])
			assert.Equal(t, wantStatus[tc.Name], check["status"])
			assert.Equal(t, float64(providertest.Now.Unix()),
				check["issued"])
			output := check["output"].(string)
			assert.Contains(t, output, tc.Message.Note)
			providertest.AssertOneLeadingEmoji(t, output)
		})
	}
}

func TestSensugoCheckStatus(t *testing.T) {
	warning := notification.Message{
		Key: "p-1", Route: notification.Route{Severity: "warning"},
	}
	assert.Equal(t, 1, checkStatus(warning))
	assert.Equal(t, 0, checkStatus(notification.Notice("x")))
	assert.Equal(t, 2, checkStatus(notification.Message{
		Key: "p-1", Status: notification.StatusCritical,
	}))
}

func TestSensugoSendMessageIsPassingNotice(t *testing.T) {
	c, rec := newTestSensugo(t)
	require.NoError(t, c.SendMessage(context.Background(), "started"))
	check := rec.Last(t).JSON(t)["check"].(map[string]any)
	// A notice must not leave a failing check open in Sensu.
	assert.Equal(t, float64(0), check["status"])
	assert.Equal(t, "started", check["output"])
}

func TestSensugoClassifiesErrors(t *testing.T) {
	c, rec := newTestSensugo(t)
	rec.Reply = func(w http.ResponseWriter, _ providertest.Request) {
		w.WriteHeader(http.StatusForbidden)
	}
	err := c.SendIncident(context.Background(), providertest.Announce())
	assert.True(t, transport.IsPermanent(err))
	c.url = "h ttp://bad"
	assert.Error(t, c.SendMessage(context.Background(), "x"))
}
