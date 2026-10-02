package splunk

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
	"github.com/abahmed/kwatch/internal/delivery/transport"
)

func newTestSplunk(t *testing.T) (*Splunk, *providertest.Recorder) {
	t.Helper()
	rec := providertest.NewRecorder(t)
	c := NewSplunk(map[string]interface{}{
		"url": rec.URL() + "/services/collector/event", "token": "tok",
		"index": "k8s", "sourcetype": "kwatch:incident",
	}, "dev", rec.Dependencies())
	require.NotNil(t, c)
	return c, rec
}

func TestSplunkSendIncidentLifecycle(t *testing.T) {
	c, rec := newTestSplunk(t)
	wantState := map[string]string{
		"announce": "firing", "update": "firing", "resolve": "resolved",
	}
	for _, tc := range providertest.Lifecycle() {
		t.Run(tc.Name, func(t *testing.T) {
			require.NoError(t, c.SendIncident(context.Background(),
				tc.Message))
			req := rec.Last(t)
			assert.Equal(t, "Splunk tok", req.Header.Get("Authorization"))
			body := req.JSON(t)
			assert.Equal(t, "k8s", body["index"])
			assert.Equal(t, "kwatch:incident", body["sourcetype"])
			ev := body["event"].(map[string]any)
			assert.Equal(t, tc.Message.AlertKey("dev"), ev["incidentKey"])
			assert.Equal(t, wantState[tc.Name], ev["state"])
			assert.Equal(t, "dev", ev["cluster"])
			assert.Equal(t, tc.Message.Short, ev["title"])
			providertest.AssertOneLeadingEmoji(t, ev["title"].(string))
			assert.Equal(t, tc.Message.Note, ev["message"])
		})
	}
}

func TestSplunkSendMessageIsPlain(t *testing.T) {
	c, rec := newTestSplunk(t)
	require.NoError(t, c.SendMessage(context.Background(), "hello"))
	ev := rec.Last(t).JSON(t)["event"].(map[string]any)
	assert.Equal(t, "hello", ev["message"])
	assert.Nil(t, ev["incidentKey"])
}

func TestSplunkClassifiesErrors(t *testing.T) {
	c, rec := newTestSplunk(t)
	rec.Reply = func(w http.ResponseWriter, _ providertest.Request) {
		w.WriteHeader(http.StatusForbidden)
	}
	err := c.SendIncident(context.Background(), providertest.Announce())
	assert.True(t, transport.IsPermanent(err))
	c.url = "h ttp://bad"
	assert.Error(t, c.SendMessage(context.Background(), "x"))
}
