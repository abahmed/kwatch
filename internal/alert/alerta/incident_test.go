package alerta

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

func newTestAlerta(t *testing.T) (*Alerta, *providertest.Recorder) {
	t.Helper()
	rec := providertest.NewRecorder(t)
	c := NewAlerta(map[string]interface{}{
		"url": rec.URL(), "apiKey": "key", "environment": "Staging",
	}, "dev", rec.Dependencies())
	require.NotNil(t, c)
	return c, rec
}

func TestAlertaSendIncidentLifecycle(t *testing.T) {
	c, rec := newTestAlerta(t)
	wantSeverity := map[string]string{
		"announce": "critical", "update": "critical", "resolve": "normal",
	}
	for _, tc := range providertest.Lifecycle() {
		t.Run(tc.Name, func(t *testing.T) {
			require.NoError(t, c.SendIncident(context.Background(),
				tc.Message))
			req := rec.Last(t)
			body := req.JSON(t)
			assert.Equal(t, "/api/alert", req.Path)
			assert.Equal(t, "Key key", req.Header.Get("Authorization"))
			assert.Equal(t, "dev/"+tc.Message.AlertKey(""), body["resource"])
			assert.Equal(t, "incident", body["event"])
			assert.Equal(t, "Staging", body["environment"])
			assert.Equal(t, wantSeverity[tc.Name], body["severity"])
			assert.Equal(t, tc.Message.Short, body["value"])
			providertest.AssertOneLeadingEmoji(t, body["value"].(string))
			assert.Contains(t, body["text"], tc.Message.Note)
			if tc.Message.Resolved() {
				assert.Equal(t, "closed", body["status"])
			} else {
				assert.Nil(t, body["status"])
			}
		})
	}
}

func TestAlertaSeverityMapping(t *testing.T) {
	cases := map[string]struct {
		m    notification.Message
		want string
	}{
		"warning route": {notification.Message{
			Key: "a", Route: notification.Route{Severity: "warning"},
		}, "warning"},
		"info route": {notification.Message{
			Key: "a", Route: notification.Route{Severity: "info"},
		}, "informational"},
		"warning status": {notification.Message{
			Key: "a", Status: notification.StatusWarning,
		}, "warning"},
		"notice":   {notification.Notice("hi"), "informational"},
		"resolved": {providertest.Resolve(), "normal"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, alertaSeverity(tc.m))
		})
	}
}

func TestAlertaSendMessageIsNotice(t *testing.T) {
	c, rec := newTestAlerta(t)
	require.NoError(t, c.SendMessage(context.Background(), "started"))
	body := rec.Last(t).JSON(t)
	assert.Equal(t, "kwatch", body["event"])
	assert.Equal(t, "dev/kwatch-notice", body["resource"])
	assert.Equal(t, "started", body["text"])
}

func TestAlertaClassifiesErrors(t *testing.T) {
	c, rec := newTestAlerta(t)
	rec.Reply = func(w http.ResponseWriter, _ providertest.Request) {
		w.WriteHeader(http.StatusForbidden)
	}
	err := c.SendIncident(context.Background(), providertest.Announce())
	assert.True(t, transport.IsPermanent(err))
	c.url = "h ttp://bad"
	assert.Error(t, c.SendMessage(context.Background(), "x"))
}

func TestAlertaUsesStableDedupKeyAsResource(t *testing.T) {
	c, rec := newTestAlerta(t)
	msg := providertest.Lifecycle()[0].Message
	msg.DedupKey = "deployment-shop-payments-crashloop"

	require.NoError(t, c.SendIncident(context.Background(), msg))

	assert.Equal(t, "dev/kwatch-deployment-shop-payments-crashloop",
		rec.Last(t).JSON(t)["resource"])
}
