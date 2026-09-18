package health

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/metrics"
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
	if response.Degraded["status"] != "component_stopped" {
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

func TestComponentErrorUsesSafeReasonAndCountsTransitions(t *testing.T) {
	server := &HealthServer{}
	before := metrics.DefaultRegistry().ComponentDegradations.Load()
	server.SetComponentError(
		"provider",
		errors.New("provider token=secret failed to send payload"),
	)
	server.SetComponentError(
		"provider",
		errors.New("provider token=another-secret failed to send payload"),
	)
	server.SetComponentStatus(
		"provider", "degraded", "rate_limited", false,
	)

	if got := server.ComponentErrors()["provider"]; got != "component_failed" {
		t.Fatalf("unexpected component error reason: %q", got)
	}
	if got := metrics.DefaultRegistry().ComponentDegradations.Load() -
		before; got != 1 {
		t.Fatalf("degradation metric changed by %d, want one transition", got)
	}
	status := server.ComponentStatuses()["provider"]
	if status.Reason != "rate_limited" {
		t.Fatalf("unexpected component status reason: %q", status.Reason)
	}
}

func TestComponentStatusRecordsOnlyTransitionTime(t *testing.T) {
	first := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	second := first.Add(time.Minute)
	now := first
	server := NewHealthServerWithClock(
		config.HealthCheck{},
		clock.Func(func() time.Time { return now }),
	)

	server.SetComponentStatus("watcher", "degraded", "cache_sync_failed", false)
	status := server.ComponentStatuses()["watcher"]
	if !status.LastTransition.Equal(first) {
		t.Fatalf("first transition = %v, want %v",
			status.LastTransition, first)
	}

	now = second
	server.SetComponentStatus("watcher", "degraded", "cache_sync_failed", false)
	status = server.ComponentStatuses()["watcher"]
	if !status.LastTransition.Equal(first) {
		t.Fatalf("repeated status changed transition time to %v",
			status.LastTransition)
	}

	server.SetComponentStatus("watcher", "running", "", true)
	status = server.ComponentStatuses()["watcher"]
	if !status.LastTransition.Equal(second) {
		t.Fatalf("recovery transition = %v, want %v",
			status.LastTransition, second)
	}
}

func TestComponentStatusSanitizesArbitraryReasons(t *testing.T) {
	server := &HealthServer{}
	server.SetComponentStatus(
		"watcher", "degraded", "token=secret https://example.invalid", false,
	)
	status := server.ComponentStatuses()["watcher"]
	if status.Reason != "component_failed" {
		t.Fatalf("reason = %q, want safe bounded reason", status.Reason)
	}
	if got := server.ComponentErrors()["watcher"]; got != "component_failed" {
		t.Fatalf("component error = %q, want safe bounded reason", got)
	}
}
