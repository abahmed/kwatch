package health

import (
	"net/http/httptest"
	"testing"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/config"
)

func metricsCode(h *HealthServer, path string) (int, string) {
	w := httptest.NewRecorder()
	newServeMux(h).ServeHTTP(w, httptest.NewRequest("GET", path, nil))
	return w.Code, w.Body.String()
}

func TestMetricsDeniedAnswersNotFound(t *testing.T) {
	h := NewHealthServerWithClock(
		config.HealthCheck{StatusEndpoint: true}, clock.RealClock{})
	h.DenyMetrics()
	if code, body := metricsCode(h, "/metrics"); code != 404 || body != "" {
		t.Fatalf("denied /metrics = %d %q, want an empty 404", code, body)
	}
	for _, path := range []string{"/healthz", "/status"} {
		if code, _ := metricsCode(h, path); code == 404 {
			t.Errorf("%s = 404, want it served", path)
		}
	}
}

func TestMetricsAllowedAfterDeny(t *testing.T) {
	h := NewHealthServerWithClock(config.HealthCheck{}, clock.RealClock{})
	h.DenyMetrics()
	h.AllowMetrics()
	if code, _ := metricsCode(h, "/metrics"); code != 200 {
		t.Fatalf("allowed /metrics = %d, want 200", code)
	}
}
