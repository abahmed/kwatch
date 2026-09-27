package signl4

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/event"
)

func TestSignl4ResolvesByExternalID(t *testing.T) {
	var body string
	s := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			raw, _ := io.ReadAll(r.Body)
			body = string(raw)
		}))
	defer s.Close()
	c := NewSignl4(map[string]interface{}{
		"teamSecret": "t", "url": s.URL,
	}, testAppConfig(), testDeps)
	assert.NoError(t, c.SendEvent(context.Background(), &event.Event{
		DedupKey: "abc", Action: "resolved", Narrative: "api recovered",
	}))
	assert.Contains(t, body, `"X-S4-Status":"resolved"`)
	assert.Contains(t, body, `"X-S4-ExternalID":"kwatch-abc"`)
}
