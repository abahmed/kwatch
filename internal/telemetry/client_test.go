package telemetry

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestShouldSend(t *testing.T) {
	now := time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)
	if !ShouldSend(time.Time{}, now) {
		t.Fatal("expected first heartbeat to be due")
	}
	if ShouldSend(now.Add(-time.Hour), now) {
		t.Fatal("expected recent heartbeat not to be due")
	}
	if !ShouldSend(now.Add(-WeeklyInterval), now) {
		t.Fatal("expected weekly heartbeat to be due")
	}
}

func TestReportSendsOnlyExpectedPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if string(body) != `{"cluster_uuid":"123e4567-e89b-42d3-a456-426614174000","kwatch_version":"v1.2.3"}` {
			t.Fatalf("unexpected payload: %s", body)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Fatal("expected JSON content type")
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	if err := Report(context.Background(), server.Client(), server.URL,
		"123e4567-e89b-42d3-a456-426614174000", "v1.2.3"); err != nil {
		t.Fatal(err)
	}
}

func TestReportRejectsInvalidIdentity(t *testing.T) {
	if err := Report(context.Background(), nil, "https://example.com", "not-a-uuid", "dev"); err == nil {
		t.Fatal("expected invalid identity error")
	}
}
