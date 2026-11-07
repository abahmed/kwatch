package dashboard

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/status"
)

const (
	testToken    = "secret-token"
	testPassword = "7K4M-QX9P-2HVD-T6RE"
)

var testEpoch = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

// fakeIncidents is an incidentReader over a fixed list.
type fakeIncidents struct {
	mu    sync.Mutex
	items []incident.Incident
}

func (f *fakeIncidents) Incidents() []incident.Incident {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]incident.Incident(nil), f.items...)
}

func (f *fakeIncidents) Incident(id string) *incident.Incident {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.items {
		if f.items[i].ID == id {
			p := f.items[i]
			return &p
		}
	}
	return nil
}

func testIncident(
	id, namespace string, opened time.Duration,
) incident.Incident {
	return incident.Incident{
		ID: id,
		Root: inventory.EntityID{Kind: "Deployment",
			Namespace: namespace, Name: "app-" + id},
		State:  incident.Open,
		Tier:   incident.Notify,
		Opened: testEpoch.Add(opened),
		// Open means announced: a message of it went out.
		Announced: testEpoch.Add(opened),
	}
}

// newTestServer is a server with every source set.
func newTestServer(items ...incident.Incident) *Server {
	s := NewServer(Options{Token: testToken, Password: testPassword,
		Edition: "Pro", Self: Self{Namespace: "kwatch",
			Deployment: "kwatch"}},
		clock.Func(func() time.Time { return testEpoch }))
	s.SetIncidents(&fakeIncidents{items: items})
	s.SetStatus(func() status.Report { return status.Report{At: testEpoch} })
	s.SetDeliveryHealth(func() []ChannelHealth {
		return []ChannelHealth{{Channel: "slack", LastSuccess: testEpoch}}
	})
	return s
}

// get calls the server's handler with the test token.
func get(t *testing.T, s *Server, path string) *httptest.ResponseRecorder {
	t.Helper()
	return call(t, s, http.MethodGet, path, testToken)
}

func call(
	t *testing.T, s *Server, method, path, token string,
) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	return w
}

// getPart is get, with the body narrowed to one part of an answer that
// carries several, such as the insights of /api/overview.
func getPart(
	t *testing.T, s *Server, path, key string,
) *httptest.ResponseRecorder {
	t.Helper()
	w := get(t, s, path)
	if w.Code != http.StatusOK {
		return w
	}
	var all map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &all); err != nil {
		t.Fatal(err)
	}
	w.Body = bytes.NewBuffer(all[key])
	return w
}

// toDetail is the detail of p alone, without the server's context.
func toDetail(p incident.Incident) IncidentDetail {
	return buildDetail(p, detailContext{})
}
