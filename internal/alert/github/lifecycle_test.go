package github

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/event"
)

func TestGithubFilesOneIssuePerIncident(t *testing.T) {
	var requests []string
	s := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			requests = append(requests,
				r.Method+" "+r.URL.Path+" "+string(body))
			if r.Method == http.MethodPost && r.URL.Path == "/repos/o/r/issues" {
				_, _ = w.Write([]byte(`{"number":12}`))
			}
		}))
	defer s.Close()
	c := NewGithub(map[string]interface{}{
		"token": "t", "owner": "o", "repo": "r", "url": s.URL,
	}, testAppConfig(), testDeps)
	ctx := context.Background()
	for _, action := range []string{"create", "update", "resolved"} {
		assert.NoError(t, c.SendEvent(ctx, &event.Event{
			DedupKey: "abc", Action: action, Narrative: "api " + action,
		}))
	}
	if assert.Len(t, requests, 4) {
		assert.Contains(t, requests[0], "POST /repos/o/r/issues ")
		assert.Contains(t, requests[1], "POST /repos/o/r/issues/12/comments")
		assert.Contains(t, requests[2], "POST /repos/o/r/issues/12/comments")
		assert.Contains(t, requests[3],
			`PATCH /repos/o/r/issues/12 {"state":"closed"}`)
	}
}
