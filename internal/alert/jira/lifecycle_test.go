package jira

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/event"
)

func TestJiraCommentsOnRecovery(t *testing.T) {
	var requests []string
	s := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			requests = append(requests, r.Method+" "+r.URL.Path)
			if r.URL.Path == "/rest/api/2/issue" {
				_, _ = w.Write([]byte(`{"key":"OPS-9"}`))
			}
		}))
	defer s.Close()
	c := NewJira(map[string]interface{}{
		"url": s.URL, "user": "u", "apiToken": "t", "projectKey": "OPS",
	}, testAppConfig(), testDeps)
	ctx := context.Background()
	for _, action := range []string{"create", "resolved"} {
		assert.NoError(t, c.SendEvent(ctx, &event.Event{
			DedupKey: "abc", Action: action, Narrative: "api " + action,
		}))
	}
	assert.Equal(t, []string{
		"POST /rest/api/2/issue",
		"POST /rest/api/2/issue/OPS-9/comment",
	}, requests)
}
