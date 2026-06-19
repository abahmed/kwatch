package health

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/stretchr/testify/assert"
)

type fakeIncidentLister struct {
	snap []model.IncidentView
}

func (f *fakeIncidentLister) Snapshot() []model.IncidentView {
	return f.snap
}

type fakeAlertSender struct {
	events []event.Event
	msgs   []string
}

func (f *fakeAlertSender) NotifyEvent(ev event.Event) {
	f.events = append(f.events, ev)
}
func (f *fakeAlertSender) Notify(msg string) {
	f.msgs = append(f.msgs, msg)
}

func TestNewHealthServer(t *testing.T) {
	assert := assert.New(t)

	server := NewHealthServer(config.HealthCheck{Port: 8080, Enabled: true})
	assert.NotNil(server)
	assert.Equal(8080, server.port)
	assert.True(server.enabled)
}

func TestNewHealthServerDisabled(t *testing.T) {
	assert := assert.New(t)

	server := NewHealthServer(config.HealthCheck{Port: 8080, Enabled: false})
	assert.NotNil(server)
	assert.Equal(8080, server.port)
	assert.False(server.enabled)
}

func TestHealthzHandler(t *testing.T) {
	assert := assert.New(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := &HealthServer{}
		h.healthzHandler(w, r)
	}))
	defer server.Close()

	resp, err := http.Get(server.URL)
	assert.Nil(err)
	assert.Equal(http.StatusOK, resp.StatusCode)

	body := make([]byte, 100)
	n, _ := resp.Body.Read(body)
	assert.Equal("OK", string(body[:n]))
}

func TestHealthHandler(t *testing.T) {
	assert := assert.New(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := &HealthServer{}
		h.healthHandler(w, r)
	}))
	defer server.Close()

	resp, err := http.Get(server.URL)
	assert.Nil(err)
	assert.Equal(http.StatusOK, resp.StatusCode)
	assert.Equal("application/json", resp.Header.Get("Content-Type"))

	var healthResp HealthResponse
	err = json.NewDecoder(resp.Body).Decode(&healthResp)
	assert.Nil(err)
	assert.Equal("ok", healthResp.Status)
}

func TestHealthServerStartDisabled(t *testing.T) {
	assert := assert.New(t)

	server := NewHealthServer(config.HealthCheck{Port: 8080, Enabled: false})
	err := server.Start(context.Background())
	assert.Nil(err)
}

func TestHealthServerStartEnabled(t *testing.T) {
	assert := assert.New(t)

	server := NewHealthServer(config.HealthCheck{Port: 8080, Enabled: true})
	err := server.Start(context.Background())
	assert.Nil(err)

	// Test /healthz endpoint
	resp, err := http.Get("http://localhost:8080/healthz")
	assert.Nil(err)
	assert.Equal(http.StatusOK, resp.StatusCode)

	// Test /health endpoint
	resp, err = http.Get("http://localhost:8080/health")
	assert.Nil(err)
	assert.Equal(http.StatusOK, resp.StatusCode)

	server.Stop(context.Background())
}

func TestHealthServerStop(t *testing.T) {
	assert := assert.New(t)

	server := NewHealthServer(config.HealthCheck{Port: 8080, Enabled: true})
	err := server.Start(context.Background())
	assert.Nil(err)

	err = server.Stop(context.Background())
	assert.Nil(err)
}

func TestHealthServerStopNilServer(t *testing.T) {
	assert := assert.New(t)

	server := NewHealthServer(config.HealthCheck{Port: 8080, Enabled: true})
	err := server.Stop(context.Background())
	assert.Nil(err)
}

func TestIncidentsHandlerNoAPI(t *testing.T) {
	h := &HealthServer{}
	req := httptest.NewRequest(http.MethodGet, "/incidents", nil)
	w := httptest.NewRecorder()
	h.incidentsHandler(w, req)

	resp := w.Result()
	assert.Equal(t, http.StatusServiceUnavailable, resp.StatusCode)
}

func TestIncidentsHandler(t *testing.T) {
	assert := assert.New(t)
	lister := &fakeIncidentLister{
		snap: []model.IncidentView{
			{
				Key:       "ns:deploy:Err",
				Reason:    "Err",
				Namespace: "ns",
				Name:      "deploy",
				Count:     1,
				State:     model.StateActive,
				FirstSeen: time.Now(),
				LastSeen:  time.Now(),
			},
		},
	}
	h := &HealthServer{incidentAPI: lister}

	req := httptest.NewRequest(http.MethodGet, "/incidents", nil)
	w := httptest.NewRecorder()
	h.incidentsHandler(w, req)

	resp := w.Result()
	assert.Equal(http.StatusOK, resp.StatusCode)
	assert.Equal("application/json", resp.Header.Get("Content-Type"))

	var got []model.IncidentView
	err := json.NewDecoder(resp.Body).Decode(&got)
	assert.Nil(err)
	assert.Len(got, 1)
	assert.Equal("ns:deploy:Err", got[0].Key)
}

func TestIncidentsHandlerEmpty(t *testing.T) {
	assert := assert.New(t)
	lister := &fakeIncidentLister{}
	h := &HealthServer{incidentAPI: lister}

	req := httptest.NewRequest(http.MethodGet, "/incidents", nil)
	w := httptest.NewRecorder()
	h.incidentsHandler(w, req)

	resp := w.Result()
	assert.Equal(http.StatusOK, resp.StatusCode)

	var got []model.IncidentView
	err := json.NewDecoder(resp.Body).Decode(&got)
	assert.Nil(err)
	assert.Len(got, 0)
}

func TestTestAlertHandlerNoAM(t *testing.T) {
	assert := assert.New(t)
	h := &HealthServer{}
	req := httptest.NewRequest(http.MethodPost, "/test-alert", nil)
	w := httptest.NewRecorder()
	h.testAlertHandler(w, req)

	resp := w.Result()
	assert.Equal(http.StatusServiceUnavailable, resp.StatusCode)
}

func TestTestAlertHandlerMethodNotAllowed(t *testing.T) {
	assert := assert.New(t)
	am := &fakeAlertSender{}
	h := &HealthServer{alertManager: am}

	req := httptest.NewRequest(http.MethodGet, "/test-alert", nil)
	w := httptest.NewRecorder()
	h.testAlertHandler(w, req)

	resp := w.Result()
	assert.Equal(http.StatusMethodNotAllowed, resp.StatusCode)
}

func TestTestAlertHandler(t *testing.T) {
	am := &fakeAlertSender{}
	h := &HealthServer{alertManager: am}

	req := httptest.NewRequest(http.MethodPost, "/test-alert", bytes.NewReader([]byte{}))
	w := httptest.NewRecorder()
	h.testAlertHandler(w, req)

	resp := w.Result()
	assert.Equal(t, http.StatusOK, resp.StatusCode)

	body := make([]byte, 100)
	n, _ := resp.Body.Read(body)
	assert.Equal(t, "test alert sent", string(body[:n]))

	if len(am.events) != 1 {
		t.Fatalf("expected 1 sent event, got %d", len(am.events))
	}
	if len(am.msgs) != 1 {
		t.Fatalf("expected 1 sent message, got %d", len(am.msgs))
	}
}

func TestRequireDiagnosticsAuthEmptyToken(t *testing.T) {
	h := &HealthServer{diagnosticsToken: ""}
	req := httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil)
	w := httptest.NewRecorder()
	assert.True(t, h.requireDiagnosticsAuth(w, req), "empty token must allow all requests")
}

func TestRequireDiagnosticsAuthValidToken(t *testing.T) {
	h := &HealthServer{diagnosticsToken: "secret123"}
	req := httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil)
	req.Header.Set("Authorization", "Bearer secret123")
	w := httptest.NewRecorder()
	assert.True(t, h.requireDiagnosticsAuth(w, req), "valid Bearer token must authenticate")
}

func TestRequireDiagnosticsAuthInvalidToken(t *testing.T) {
	h := &HealthServer{diagnosticsToken: "secret123"}
	req := httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	w := httptest.NewRecorder()
	assert.False(t, h.requireDiagnosticsAuth(w, req), "wrong token must reject")

	resp := w.Result()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)

	body := make([]byte, 32)
	n, _ := resp.Body.Read(body)
	assert.Equal(t, "unauthorized", string(body[:n]))
}

func TestRequireDiagnosticsAuthMissingHeader(t *testing.T) {
	h := &HealthServer{diagnosticsToken: "secret123"}
	req := httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil)
	w := httptest.NewRecorder()
	assert.False(t, h.requireDiagnosticsAuth(w, req), "missing Authorization must reject")

	resp := w.Result()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestGuardWithoutTokenCallsHandler(t *testing.T) {
	h := &HealthServer{diagnosticsToken: ""}
	called := false
	handler := h.guard(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})
	req := httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil)
	w := httptest.NewRecorder()
	handler(w, req)
	assert.True(t, called, "handler must be called when no token is set")
}

func TestGuardWithValidTokenCallsHandler(t *testing.T) {
	h := &HealthServer{diagnosticsToken: "secret123"}
	called := false
	handler := h.guard(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})
	req := httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil)
	req.Header.Set("Authorization", "Bearer secret123")
	w := httptest.NewRecorder()
	handler(w, req)
	assert.True(t, called, "handler must be called with valid token")
}

func TestGuardWithInvalidTokenReturns401(t *testing.T) {
	h := &HealthServer{diagnosticsToken: "secret123"}
	handler := h.guard(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler must not be called with invalid token")
	})
	req := httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	w := httptest.NewRecorder()
	handler(w, req)

	resp := w.Result()
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}

func TestPprofEndpointsRegisteredWithGuard(t *testing.T) {
	h := &HealthServer{diagnostics: true, pprof: true, diagnosticsToken: "tok"}
	h.SetIncidentAPI(&fakeIncidentLister{snap: []model.IncidentView{}})
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", h.healthzHandler)
	mux.HandleFunc("/incidents", h.incidentsHandler)
	if h.pprof {
		mux.HandleFunc("/debug/pprof/", h.guard(http.NotFound))
	}
	ts := httptest.NewServer(mux)
	defer ts.Close()

	// Without token — pprof should reject
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/debug/pprof/", nil)
	resp, err := http.DefaultClient.Do(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	resp.Body.Close()

	// Without token — non-pprof endpoints still work
	req, _ = http.NewRequest(http.MethodGet, ts.URL+"/healthz", nil)
	resp, err = http.DefaultClient.Do(req)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()
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
