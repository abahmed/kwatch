package goalert

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/event"
)

func TestGoalertClosesResolvedIncident(t *testing.T) {
	var body string
	s := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			raw, _ := io.ReadAll(r.Body)
			body = string(raw)
		}))
	defer s.Close()
	c := NewGoalert(map[string]interface{}{
		"token": "t", "serviceId": "S", "url": s.URL,
	}, testAppConfig(), testDeps)
	assert.NoError(t, c.SendEvent(context.Background(), &event.Event{
		DedupKey: "abc", Action: "resolved", Narrative: "api recovered",
	}))
	assert.Contains(t, body, `"action":"close"`)
	assert.Contains(t, body, `"dedup":"kwatch-abc"`)
}
