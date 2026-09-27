package splunkoncall

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/event"
)

func TestSplunkOncallIncidentLifecycle(t *testing.T) {
	var bodies []string
	s := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			bodies = append(bodies, string(body))
		}))
	defer s.Close()
	c := NewSplunkOncall(map[string]interface{}{
		"apiKey": "k", "routingKey": "r",
	}, testAppConfig(), testDeps)
	c.url = s.URL
	ctx := context.Background()
	assert.NoError(t, c.SendEvent(ctx, &event.Event{
		DedupKey: "abc", Action: "create", Narrative: "api crashed",
	}))
	assert.NoError(t, c.SendEvent(ctx, &event.Event{
		DedupKey: "abc", Action: "resolved", Narrative: "api recovered",
	}))
	assert.Contains(t, bodies[0], `"message_type":"CRITICAL"`)
	assert.Contains(t, bodies[0], `"entity_id":"kwatch-abc"`)
	assert.Contains(t, bodies[1], `"message_type":"RECOVERY"`)
	assert.Contains(t, bodies[1], `"entity_id":"kwatch-abc"`)
}
