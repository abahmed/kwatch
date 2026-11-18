package health

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
)

func TestHealthShowsTheHeartbeatWithoutTurningRedWhenOff(t *testing.T) {
	server := NewHealthServerWithClock(
		config.HealthCheck{}, clock.RealClock{})
	server.SetHeartbeat(HeartbeatStatus{State: HeartbeatNotConfigured})

	body := healthBody(t, server)

	if body.Status != "ok" {
		t.Fatalf("status = %q, want ok", body.Status)
	}
	if body.Heartbeat == nil ||
		body.Heartbeat.State != HeartbeatNotConfigured {
		t.Fatalf("heartbeat = %+v", body.Heartbeat)
	}
	c := body.Components[HeartbeatComponent]
	if c.State != "off" || c.Reason != "heartbeat_not_configured" ||
		!c.Available {
		t.Fatalf("component = %+v", c)
	}
}

func TestHealthHeartbeatFailingIsDegraded(t *testing.T) {
	server := NewHealthServerWithClock(
		config.HealthCheck{}, clock.RealClock{})
	at := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	server.SetHeartbeat(HeartbeatStatus{State: HeartbeatFailing,
		LastFailure: at, Failure: HeartbeatRejected, HTTPStatus: 404})

	body := healthBody(t, server)

	if body.Status != "degraded" ||
		body.Degraded[HeartbeatComponent] != "heartbeat_failed" {
		t.Fatalf("body = %+v", body)
	}
	if body.Heartbeat.HTTPStatus != 404 {
		t.Fatalf("heartbeat = %+v", body.Heartbeat)
	}
	server.SetHeartbeat(HeartbeatStatus{State: HeartbeatOK, LastPing: at})
	if body := healthBody(t, server); body.Status != "ok" {
		t.Fatalf("recovered status = %q", body.Status)
	}
}

func healthBody(t *testing.T, server *HealthServer) HealthResponse {
	t.Helper()
	rec := httptest.NewRecorder()
	server.healthHandler(rec, httptest.NewRequest("GET", "/health", nil))
	var body HealthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body
}
