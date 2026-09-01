package health

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/model"
)

func TestSetDeadLetterLister(t *testing.T) {
	h := &HealthServer{}
	assert.Nil(t, h.deadLetterLister)
	h.SetDeadLetterLister(&fakeDeadLetterLister{letters: []string{"a"}})
	assert.NotNil(t, h.deadLetterLister)
}

func TestSetReady(t *testing.T) {
	h := &HealthServer{}
	assert.False(t, h.ready.Load())
	h.SetReady(true)
	assert.True(t, h.ready.Load())
	h.SetReady(false)
	assert.False(t, h.ready.Load())
}

func TestReadyzHandlerNotReady(t *testing.T) {
	h := &HealthServer{}
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()
	h.readyzHandler(w, req)
	resp := w.Result()
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
	body := make([]byte, 32)
	n, _ := resp.Body.Read(body)
	assert.Equal(t, "not ready", string(body[:n]))
}

func TestReadyzHandlerReady(t *testing.T) {
	h := &HealthServer{}
	h.SetReady(true)
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()
	h.readyzHandler(w, req)
	resp := w.Result()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	body := make([]byte, 8)
	n, _ := resp.Body.Read(body)
	assert.Equal(t, "OK", string(body[:n]))
}

func TestDeadLettersHandlerNoLister(t *testing.T) {
	h := &HealthServer{}
	req := httptest.NewRequest(http.MethodGet, "/deadletters", nil)
	w := httptest.NewRecorder()
	h.deadLettersHandler(w, req)
	resp := w.Result()
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
}

func TestDeadLettersHandlerWithData(t *testing.T) {
	expected := map[string]string{"key": "value"}
	h := &HealthServer{deadLetterLister: &fakeDeadLetterLister{letters: expected}}
	req := httptest.NewRequest(http.MethodGet, "/deadletters", nil)
	w := httptest.NewRecorder()
	h.deadLettersHandler(w, req)
	resp := w.Result()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))
	var got map[string]string
	err := json.NewDecoder(resp.Body).Decode(&got)
	assert.Nil(t, err)
	assert.Equal(t, expected, got)
}

func TestDeadLettersHandlerAuthFails(t *testing.T) {
	h := &HealthServer{diagnosticsToken: "secret"}
	req := httptest.NewRequest(http.MethodGet, "/deadletters", nil)
	w := httptest.NewRecorder()
	h.deadLettersHandler(w, req)
	resp := w.Result()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestPprofEndpointsNotRegisteredWhenDisabled(t *testing.T) {
	h := &HealthServer{pprof: false}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", h.healthzHandler)
	// pprof NOT registered
	ts := httptest.NewServer(mux)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/debug/pprof/")
	assert.Nil(t, err)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	resp.Body.Close()
}

func TestDiagnosticsDisabled(t *testing.T) {
	h := &HealthServer{diagnostics: false}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", h.healthzHandler)
	mux.HandleFunc("/health", h.healthHandler)
	mux.HandleFunc("/readyz", h.readyzHandler)
	// /incidents and /test-alert NOT registered when diagnostics is false

	ts := httptest.NewServer(mux)
	defer ts.Close()

	// /healthz always works
	resp, err := http.Get(ts.URL + "/healthz")
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// /incidents returns 404 when diagnostics disabled
	resp, err = http.Get(ts.URL + "/incidents")
	assert.Nil(t, err)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	resp.Body.Close()

	// /test-alert returns 404 when diagnostics disabled
	resp, err = http.Post(ts.URL+"/test-alert", "text/plain", nil)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusNotFound, resp.StatusCode)
	resp.Body.Close()
}

func TestDiagnosticsEnabled(t *testing.T) {
	h := &HealthServer{diagnostics: true}
	h.SetIncidentAPI(&fakeIncidentLister{snap: []model.IncidentView{}})
	h.SetAlertManager(&fakeAlertSender{})
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", h.healthzHandler)
	mux.HandleFunc("/health", h.healthHandler)
	mux.HandleFunc("/readyz", h.readyzHandler)
	mux.HandleFunc("/incidents", h.incidentsHandler)
	mux.HandleFunc("/test-alert", h.testAlertHandler)

	ts := httptest.NewServer(mux)
	defer ts.Close()

	// /healthz always works
	resp, err := http.Get(ts.URL + "/healthz")
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// /incidents returns 200 when diagnostics enabled
	resp, err = http.Get(ts.URL + "/incidents")
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// /test-alert returns 200 when diagnostics enabled
	resp, err = http.Post(ts.URL+"/test-alert", "text/plain", nil)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()
}
