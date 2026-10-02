package signl4

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

func newTestSignl4(
	t *testing.T, extra map[string]interface{},
) (*Signl4, *providertest.Recorder) {
	t.Helper()
	rec := providertest.NewRecorder(t)
	config := map[string]interface{}{
		"teamSecret": "secret", "url": rec.URL(), "user": "kwatch",
	}
	for name, value := range extra {
		config[name] = value
	}
	c := NewSignl4(config, "dev", rec.Dependencies())
	require.NotNil(t, c)
	return c, rec
}

func TestSignl4SendIncidentLifecycle(t *testing.T) {
	c, rec := newTestSignl4(t, nil)
	wantStatus := map[string]string{
		"announce": "new", "update": "new", "resolve": "resolved",
	}
	for _, tc := range providertest.Lifecycle() {
		t.Run(tc.Name, func(t *testing.T) {
			require.NoError(t, c.SendIncident(context.Background(),
				tc.Message))
			req := rec.Last(t)
			body := req.JSON(t)
			assert.Equal(t, "/secret", req.Path)
			assert.Equal(t, tc.Message.AlertKey("dev"), body["X-S4-ExternalID"])
			assert.Equal(t, wantStatus[tc.Name], body["X-S4-Status"])
			assert.Equal(t, tc.Message.Short, body["title"])
			providertest.AssertOneLeadingEmoji(t, body["title"].(string))
			assert.Contains(t, body["message"], tc.Message.Note)
			assert.Contains(t, body["message"], "Cluster: dev")
			assert.Equal(t, "critical", body["severity"])
			assert.Equal(t, "kwatch", body["user"])
		})
	}
}

func TestSignl4CustomTitleAndTruncation(t *testing.T) {
	c, rec := newTestSignl4(t, map[string]interface{}{"title": "Custom"})
	require.NoError(t, c.SendIncident(context.Background(),
		providertest.Announce()))
	assert.Equal(t, "Custom", rec.Last(t).JSON(t)["title"])

	c, rec = newTestSignl4(t, nil)
	m := providertest.Announce()
	m.Short = "🔴 " + strings.Repeat("x", 400)
	require.NoError(t, c.SendIncident(context.Background(), m))
	title := rec.Last(t).JSON(t)["title"].(string)
	assert.LessOrEqual(t, len(title), maxTitleBytes)
}

func TestSignl4ClassifiesErrors(t *testing.T) {
	c, rec := newTestSignl4(t, nil)
	rec.Reply = func(w http.ResponseWriter, _ providertest.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}
	err := c.SendIncident(context.Background(), providertest.Announce())
	assert.True(t, transport.IsPermanent(err))

	c.url = "h ttp://bad"
	assert.Error(t, c.SendIncident(context.Background(), providertest.Announce()))
}

func TestSignl4SkipsNotices(t *testing.T) {
	c, rec := newTestSignl4(t, nil)
	providertest.AssertNoticesSkipped(t, c, rec)
}
