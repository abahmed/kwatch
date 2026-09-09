package health

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

// A failed optional monitor must not make kwatch NotReady.
//
// This test previously asserted the opposite. Every component that reports an
// error here is an optional monitor -- the generic status watcher, the graph
// builders, runtime metrics, the control-plane probe -- and any of them fails
// simply because a cluster does not serve that API. Failing readiness for one
// made Kubernetes restart a kwatch that was watching the cluster correctly,
// forever. The failure is reported through /health instead.
func TestOptionalComponentErrorKeepsReadiness(t *testing.T) {
	server := &HealthServer{}
	server.SetReady(true)
	server.SetComponentError("status", errors.New("stopped"))

	recorder := httptest.NewRecorder()
	server.readyzHandler(
		recorder,
		httptest.NewRequest(http.MethodGet, "/readyz", nil),
	)
	if recorder.Code != http.StatusOK {
		t.Fatalf("optional component failure changed readiness: %d",
			recorder.Code)
	}

	recorder = httptest.NewRecorder()
	server.healthHandler(
		recorder,
		httptest.NewRequest(http.MethodGet, "/health", nil),
	)
	var response HealthResponse
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode health response: %v", err)
	}
	if response.Status != "degraded" {
		t.Fatalf("health did not report degraded: %q", response.Status)
	}
	if response.Degraded["status"] != "stopped" {
		t.Fatalf("health did not name the failed component: %v",
			response.Degraded)
	}

	server.SetComponentError("status", nil)
	recorder = httptest.NewRecorder()
	server.healthHandler(
		recorder,
		httptest.NewRequest(http.MethodGet, "/health", nil),
	)
	response = HealthResponse{}
	if err := json.NewDecoder(recorder.Body).Decode(&response); err != nil {
		t.Fatalf("decode health response: %v", err)
	}
	if response.Status != "ok" || len(response.Degraded) != 0 {
		t.Fatalf("recovery did not clear the degraded report: %+v", response)
	}
}

// Readiness follows the controller: until the informer caches have synced,
// kwatch is not watching anything and must say so.
func TestReadinessFollowsController(t *testing.T) {
	server := &HealthServer{}
	recorder := httptest.NewRecorder()
	server.readyzHandler(
		recorder,
		httptest.NewRequest(http.MethodGet, "/readyz", nil),
	)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("unready server reported ready: %d", recorder.Code)
	}
	server.SetReady(true)
	recorder = httptest.NewRecorder()
	server.readyzHandler(
		recorder,
		httptest.NewRequest(http.MethodGet, "/readyz", nil),
	)
	if recorder.Code != http.StatusOK {
		t.Fatalf("ready server reported unready: %d", recorder.Code)
	}
}
