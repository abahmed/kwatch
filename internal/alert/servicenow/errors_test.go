package servicenow

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/ratelimit"
)

func answerWith(status int, header map[string]string) func(
	http.ResponseWriter, *http.Request) bool {
	return func(w http.ResponseWriter, _ *http.Request) bool {
		for k, v := range header {
			w.Header().Set(k, v)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"error":{"message":"refused"}}`))
		return true
	}
}

// Credentials, roles and a wrong table never get better on retry; an
// outage or a rate limit does.
func TestServiceNowClassifiesAnswers(t *testing.T) {
	cases := []struct {
		status    int
		permanent bool
	}{
		{http.StatusBadRequest, true},
		{http.StatusUnauthorized, true},
		{http.StatusForbidden, true},
		{http.StatusNotFound, true},
		{http.StatusInternalServerError, false},
		{http.StatusServiceUnavailable, false},
	}
	for _, tc := range cases {
		f := newFakeInstance(t)
		f.setFail(answerWith(tc.status, nil))
		err := f.provider(nil).SendIncident(context.Background(),
			providertest.Announce())
		if err == nil {
			t.Fatalf("%d: no error", tc.status)
		}
		if transport.IsPermanent(err) != tc.permanent {
			t.Errorf("%d: permanent = %v, want %v", tc.status,
				transport.IsPermanent(err), tc.permanent)
		}
		if code, _ := transport.StatusOf(err); code != tc.status {
			t.Errorf("%d: status = %d", tc.status, code)
		}
		if strings.Contains(err.Error(), testPassword) {
			t.Errorf("%d: error leaks the password", tc.status)
		}
	}
}

func TestServiceNowHonorsRetryAfter(t *testing.T) {
	f := newFakeInstance(t)
	f.setFail(answerWith(http.StatusTooManyRequests,
		map[string]string{"Retry-After": "30"}))
	err := f.provider(nil).SendIncident(context.Background(),
		providertest.Announce())
	var limited *ratelimit.Error
	if !errors.As(err, &limited) {
		t.Fatalf("err = %v, want a rate limit", err)
	}
	if limited.RetryAfter != 30*time.Second {
		t.Fatalf("retry after = %s", limited.RetryAfter)
	}
	if transport.IsPermanent(err) {
		t.Fatal("a rate limit must be retried")
	}
}

func TestServiceNowTimeoutIsRetried(t *testing.T) {
	f := newFakeInstance(t)
	f.setFail(func(_ http.ResponseWriter, r *http.Request) bool {
		<-r.Context().Done()
		return true
	})
	ctx, cancel := context.WithTimeout(context.Background(),
		50*time.Millisecond)
	defer cancel()
	err := f.provider(nil).SendIncident(ctx, providertest.Announce())
	if err == nil || transport.IsPermanent(err) {
		t.Fatalf("err = %v, want a retryable timeout", err)
	}
	if f.count() != 0 {
		t.Fatal("nothing should be filed")
	}
}

// A 200 that is not a Table API answer (a hibernating instance, a proxy
// login page) files nothing and is retried.
func TestServiceNowNonJSONAnswerIsRetried(t *testing.T) {
	f := newFakeInstance(t)
	f.setFail(func(w http.ResponseWriter, _ *http.Request) bool {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html>Instance hibernating</html>"))
		return true
	})
	err := f.provider(nil).SendIncident(context.Background(),
		providertest.Announce())
	if !errors.Is(err, errNoRecord) || transport.IsPermanent(err) {
		t.Fatalf("err = %v, want a retryable errNoRecord", err)
	}
}

func TestServiceNowOAuthFetchesAndReusesToken(t *testing.T) {
	f := newFakeInstance(t)
	p := f.provider(map[string]interface{}{
		"user": "", "password": "",
		"clientID": "kwatch-client", "clientSecret": testPassword,
	})
	var auth []string
	f.setFail(func(_ http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path != tokenPath {
			auth = append(auth, r.Header.Get("Authorization"))
		}
		return false
	})
	send(t, p, providertest.Announce())
	send(t, p, providertest.Update())
	if f.tokenCount() != 1 {
		t.Fatalf("fetched %d tokens, want 1", f.tokenCount())
	}
	for _, a := range auth {
		if a != "Bearer tok-1" {
			t.Fatalf("authorization = %q", a)
		}
	}
}

// A token the instance revoked early is fetched again once.
func TestServiceNowOAuthRefetchesRefusedToken(t *testing.T) {
	f := newFakeInstance(t)
	p := f.provider(map[string]interface{}{
		"user": "", "password": "",
		"clientID": "kwatch-client", "clientSecret": testPassword,
	})
	f.setFail(func(w http.ResponseWriter, r *http.Request) bool {
		if r.Header.Get("Authorization") == "Bearer tok-1" {
			w.WriteHeader(http.StatusUnauthorized)
			return true
		}
		return false
	})
	send(t, p, providertest.Announce())
	if f.tokenCount() != 2 || f.count() != 1 {
		t.Fatalf("tokens %d incidents %d", f.tokenCount(), f.count())
	}
}

func TestServiceNowOAuthRefusedClientIsPermanent(t *testing.T) {
	f := newFakeInstance(t)
	p := f.provider(map[string]interface{}{
		"user": "", "password": "",
		"clientID": "kwatch-client", "clientSecret": testPassword,
	})
	f.setFail(func(w http.ResponseWriter, r *http.Request) bool {
		if r.URL.Path == tokenPath {
			w.WriteHeader(http.StatusUnauthorized)
			return true
		}
		return false
	})
	err := p.SendIncident(context.Background(), providertest.Announce())
	if !transport.IsPermanent(err) {
		t.Fatalf("err = %v, want permanent", err)
	}
	// The lookup and the create each ask for a token; no table request
	// goes out without one.
	for _, call := range f.calls() {
		if call != "POST "+tokenPath {
			t.Fatalf("calls = %v, want token requests only", f.calls())
		}
	}
}
