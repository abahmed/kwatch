package health

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestComponentErrorMakesReadinessFail(t *testing.T) {
	server := &HealthServer{}
	server.SetReady(true)
	server.SetComponentError("status", errors.New("stopped"))
	recorder := httptest.NewRecorder()
	server.readyzHandler(
		recorder,
		httptest.NewRequest(http.MethodGet, "/readyz", nil),
	)
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("unexpected readiness status: %d", recorder.Code)
	}
	server.SetComponentError("status", nil)
	recorder = httptest.NewRecorder()
	server.readyzHandler(
		recorder,
		httptest.NewRequest(http.MethodGet, "/readyz", nil),
	)
	if recorder.Code != http.StatusOK {
		t.Fatalf("component recovery did not restore readiness: %d", recorder.Code)
	}
}
