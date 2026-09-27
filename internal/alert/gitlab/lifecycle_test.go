package gitlab

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/event"
)

func TestGitlabClosesIssueForGroupProject(t *testing.T) {
	var requests []string
	s := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			requests = append(requests, r.Method+" "+r.URL.EscapedPath())
			if r.Method == http.MethodPost &&
				r.URL.EscapedPath() == "/projects/grp%2Fapp/issues" {
				_, _ = w.Write([]byte(`{"iid":3}`))
			}
		}))
	defer s.Close()
	c := NewGitlab(map[string]interface{}{
		"token": "t", "projectId": "grp/app", "url": s.URL,
	}, testAppConfig(), testDeps)
	ctx := context.Background()
	for _, action := range []string{"create", "resolved"} {
		assert.NoError(t, c.SendEvent(ctx, &event.Event{
			DedupKey: "abc", Action: action, Narrative: "api " + action,
		}))
	}
	assert.Equal(t, []string{
		"POST /projects/grp%2Fapp/issues",
		"POST /projects/grp%2Fapp/issues/3/notes",
		"PUT /projects/grp%2Fapp/issues/3",
	}, requests)
}
