package incidentio

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/event"
)

func TestResolveClosesTheIncidentAlert(t *testing.T) {
	var body string
	s := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			raw, _ := io.ReadAll(r.Body)
			body = string(raw)
		}))
	defer s.Close()
	c := NewIncidentio(
		map[string]interface{}{"url": "https://i"},
		testAppConfig(), testDeps,
	)
	c.url = s.URL
	assert.NoError(t, c.SendEvent(context.Background(), &event.Event{
		DedupKey: "abc", Action: "resolved", Narrative: "api recovered",
	}))
	assert.Contains(t, body, `"status":"resolved"`)
	assert.Contains(t, body, `"deduplication_key":"kwatch-abc"`)
}
