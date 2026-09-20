package teams

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSendMessageBadRequestWithBody(t *testing.T) {
	configMap := map[string]interface{}{
		"webhook": "http://example.com",
	}
	teams := NewTeams(configMap, testAppConfig(), testDeps)

	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error": "bad request details"}`))
		}))
	defer server.Close()

	teams.webhook = server.URL
	err := teams.SendMessage(context.Background(), "test message")
	if err == nil {
		t.Fatal("expected a bad request error")
	}
}

func TestSendMessage202AcceptedSucceeds(t *testing.T) {
	configMap := map[string]interface{}{
		"webhook":    "http://example.com",
		"maxRetries": 3,
		"retryDelay": 1,
	}
	teams := NewTeams(configMap, testAppConfig(), testDeps)

	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusAccepted)
		}))
	defer server.Close()

	teams.webhook = server.URL
	err := teams.SendMessage(context.Background(), "test message")
	if err != nil {
		t.Fatalf("202 Accepted should succeed: %v", err)
	}
}
