package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAuthValidToken(t *testing.T) {
	auth := NewAuth("secret-token")
	handler := auth.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	}))
	req := httptest.NewRequest("GET", "/api/incidents", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestAuthMissingToken(t *testing.T) {
	auth := NewAuth("secret-token")
	handler := auth.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	}))
	req := httptest.NewRequest("GET", "/api/incidents", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestAuthInvalidToken(t *testing.T) {
	auth := NewAuth("secret-token")
	handler := auth.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	}))
	req := httptest.NewRequest("GET", "/api/incidents", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestAuthRateLimit(t *testing.T) {
	auth := NewAuth("secret-token")
	handler := auth.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	}))
	// Exhaust the rate limiter with failed attempts
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest("GET", "/api/incidents", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("attempt %d: status = %d, want %d", i, w.Code, http.StatusUnauthorized)
		}
	}
	// Now even a valid token should be rate limited
	req := httptest.NewRequest("GET", "/api/incidents", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("status = %d, want %d", w.Code, http.StatusTooManyRequests)
	}
}

func TestAuthEmptyTokenRejectsAll(t *testing.T) {
	auth := NewAuth("")
	handler := auth.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	}))
	req := httptest.NewRequest("GET", "/api/incidents", nil)
	req.Header.Set("Authorization", "Bearer anything")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestIncidentListItemJSON(t *testing.T) {
	item := IncidentListItem{
		ID:          "inc-20261001-0001",
		Title:       "Deployment my-app is crash-looping",
		Severity:    "page",
		State:       "open",
		Namespace:   "default",
		Workload:    "my-app",
		FirstSeen:   time.Unix(100, 0),
		LastSeen:    time.Unix(200, 0),
		RepeatCount: 3,
		AckedBy:     "alice",
		ResolvedAt:  time.Unix(300, 0),
	}
	body, err := json.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}
	s := string(body)
	if !strings.Contains(s, `"id":"inc-20261001-0001"`) {
		t.Errorf("missing id field: %s", s)
	}
	if !strings.Contains(s, `"repeatCount":3`) {
		t.Errorf("missing repeatCount field: %s", s)
	}
}

func TestStatusResponseJSON(t *testing.T) {
	resp := StatusResponse{
		Version:           "v1.2.0",
		Edition:           "Pro",
		LastScan:          time.Unix(100, 0),
		WatchedNamespaces: []string{"default", "kube-system"},
		PerChannelHealth: []ChannelHealth{
			{Channel: "slack", LastSuccess: time.Unix(200, 0)},
		},
	}
	body, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}
	s := string(body)
	if !strings.Contains(s, `"edition":"Pro"`) {
		t.Errorf("missing edition field: %s", s)
	}
	if !strings.Contains(s, `"slack"`) {
		t.Errorf("missing channel field: %s", s)
	}
}
