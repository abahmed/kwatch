package clickup

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/event"
)

func TestClickupLifecycleCreateTracksThreadID(t *testing.T) {
	var createRequests int
	s := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost &&
				r.URL.Path == "/list/abc123/task" {
				createRequests++
				_, _ = w.Write([]byte(`{"id":"t1"}`))
				return
			}
			w.WriteHeader(http.StatusOK)
		}))
	defer s.Close()

	c := NewClickup(map[string]interface{}{
		"token": "t", "listId": "abc123",
	}, testAppConfig(), testDeps)
	c.url = s.URL + "/list/abc123/task"

	ctx := context.Background()
	assert.NoError(t, c.SendEvent(ctx, &event.Event{
		DedupKey: "abc", Action: "create", Narrative: "clickup create",
	}))
	assert.Equal(t, 1, createRequests)

	threads := c.SnapshotThreads()
	assert.Equal(t, "t1", threads["kwatch-abc"])
}
