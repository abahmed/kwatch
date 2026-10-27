package dashboard

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/abahmed/kwatch/internal/clock"
)

func TestAuthValidToken(t *testing.T) {
	auth := NewAuth("secret-token", clock.RealClock{})
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})
	handler := auth.Middleware(next)
	req := httptest.NewRequest("GET", "/api/incidents", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestAuthMissingToken(t *testing.T) {
	auth := NewAuth("secret-token", clock.RealClock{})
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})
	handler := auth.Middleware(next)
	req := httptest.NewRequest("GET", "/api/incidents", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestAuthInvalidToken(t *testing.T) {
	auth := NewAuth("secret-token", clock.RealClock{})
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})
	handler := auth.Middleware(next)
	req := httptest.NewRequest("GET", "/api/incidents", nil)
	req.Header.Set("Authorization", "Bearer wrong-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestAuthRateLimit(t *testing.T) {
	auth := NewAuth("secret-token", clock.RealClock{})
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})
	handler := auth.Middleware(next)
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest("GET", "/api/incidents", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("attempt %d: status = %d", i, w.Code)
		}
	}
	req := httptest.NewRequest("GET", "/api/incidents", nil)
	req.Header.Set("Authorization", "Bearer secret-token")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("status = %d, want %d", w.Code, http.StatusTooManyRequests)
	}
}

func TestAuthEmptyTokenRejectsAll(t *testing.T) {
	auth := NewAuth("", clock.RealClock{})
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})
	handler := auth.Middleware(next)
	req := httptest.NewRequest("GET", "/api/incidents", nil)
	req.Header.Set("Authorization", "Bearer anything")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestAuthRateLimitFollowsTheHostAcrossConnections(t *testing.T) {
	handler := NewAuth("secret-token", clock.RealClock{}).Middleware(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte("ok"))
		}))
	for port := range 10 {
		req := httptest.NewRequest("GET", "/api/incidents", nil)
		req.RemoteAddr = "10.1.2.3:" + strconv.Itoa(40000+port)
		handler.ServeHTTP(httptest.NewRecorder(), req)
	}

	limited := httptest.NewRequest("GET", "/api/incidents", nil)
	limited.RemoteAddr = "10.1.2.3:50000"
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, limited)
	other := httptest.NewRequest("GET", "/api/incidents", nil)
	other.RemoteAddr = "10.9.9.9:40000"
	other.Header.Set("Authorization", "Bearer secret-token")
	ok := httptest.NewRecorder()
	handler.ServeHTTP(ok, other)

	if w.Code != http.StatusTooManyRequests {
		t.Errorf("same host, new port: status = %d, want 429", w.Code)
	}
	if ok.Code != http.StatusOK {
		t.Errorf("other host: status = %d, want 200", ok.Code)
	}
}
