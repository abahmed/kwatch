package datadog

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/notification"
)

func newTestDatadog(
	t *testing.T, extra map[string]interface{},
) (*Datadog, *providertest.Recorder) {
	t.Helper()
	rec := providertest.NewRecorder(t)
	config := map[string]interface{}{
		"apiKey": "api", "applicationKey": "app",
	}
	for name, value := range extra {
		config[name] = value
	}
	c := NewDatadog(config, "dev", rec.Dependencies())
	require.NotNil(t, c)
	c.url = rec.URL() + "/api/v1/events"
	return c, rec
}

func TestDatadogSendIncidentLifecycle(t *testing.T) {
	c, rec := newTestDatadog(t, nil)
	wantType := map[string]string{
		"announce": "error", "update": "error", "resolve": "success",
	}
	for _, tc := range providertest.Lifecycle() {
		t.Run(tc.Name, func(t *testing.T) {
			require.NoError(t, c.SendIncident(context.Background(),
				tc.Message))
			req := rec.Last(t)
			body := req.JSON(t)
			assert.Equal(t, "api", req.Header.Get("DD-API-KEY"))
			assert.Equal(t, "app", req.Header.Get("DD-APPLICATION-KEY"))
			assert.Equal(t, tc.Message.AlertKey("dev"), body["aggregation_key"])
			assert.Equal(t, wantType[tc.Name], body["alert_type"])
			assert.Equal(t, tc.Message.Short, body["title"])
			providertest.AssertOneLeadingEmoji(t, body["title"].(string))
			assert.Contains(t, body["text"], tc.Message.Note)
		})
	}
}

func TestDatadogAlertTypeMapping(t *testing.T) {
	c, _ := newTestDatadog(t, nil)
	warning := notification.Message{
		Key: "a", Route: notification.Route{Severity: "warning"},
	}
	info := notification.Message{
		Key: "a", Route: notification.Route{Severity: "info"},
	}
	assert.Equal(t, "warning", c.alertTypeFor(warning))
	assert.Equal(t, "info", c.alertTypeFor(info))
	assert.Equal(t, "info", c.alertTypeFor(notification.Notice("x")))

	c, _ = newTestDatadog(t, map[string]interface{}{"alertType": "warning"})
	assert.Equal(t, "warning", c.alertTypeFor(providertest.Announce()))
	assert.Equal(t, "success", c.alertTypeFor(providertest.Resolve()))
}

func TestDatadogTruncatesTitleAndText(t *testing.T) {
	c, rec := newTestDatadog(t, nil)
	m := providertest.Announce()
	m.Short = "🔴 " + strings.Repeat("x", 300)
	m.Note = "🔴 " + strings.Repeat("y", 5000)
	require.NoError(t, c.SendIncident(context.Background(), m))
	body := rec.Last(t).JSON(t)
	assert.LessOrEqual(t, len(body["title"].(string)), maxTitleBytes)
	assert.LessOrEqual(t, len(body["text"].(string)), maxTextBytes)
}

func TestDatadogCustomTitleAndNotice(t *testing.T) {
	c, rec := newTestDatadog(t, map[string]interface{}{"title": "Custom"})
	require.NoError(t, c.SendMessage(context.Background(), "started"))
	body := rec.Last(t).JSON(t)
	assert.Equal(t, "Custom", body["title"])
	assert.Equal(t, "started", body["text"])
	assert.Equal(t, "kwatch-dev-notice", body["aggregation_key"])
}

func TestDatadogClassifiesErrors(t *testing.T) {
	c, rec := newTestDatadog(t, nil)
	rec.Reply = func(w http.ResponseWriter, _ providertest.Request) {
		w.WriteHeader(http.StatusForbidden)
	}
	err := c.SendIncident(context.Background(), providertest.Announce())
	assert.True(t, transport.IsPermanent(err))
	c.url = "h ttp://bad"
	assert.Error(t, c.SendMessage(context.Background(), "x"))
}

func TestDatadogConfiguredAlertTypeIsValidated(t *testing.T) {
	tests := []struct {
		name, alertType, want string
	}{
		{name: "valid type is kept", alertType: "error", want: "error"},
		{name: "unknown type is ignored", alertType: "critical", want: ""},
		{name: "unset stays empty", alertType: "", want: ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := newTestDatadog(t, map[string]interface{}{
				"alertType": tc.alertType,
			})
			assert.Equal(t, tc.want, c.alertType)
		})
	}
}

func TestDatadogConfiguredTitleIsTruncated(t *testing.T) {
	c, rec := newTestDatadog(t, map[string]interface{}{
		"title": strings.Repeat("t", 3*maxTitleBytes),
	})
	require.NoError(t, c.SendIncident(
		context.Background(), providertest.Announce()))
	title := rec.Last(t).JSON(t)["title"].(string)
	assert.LessOrEqual(t, len(title), maxTitleBytes)
}
