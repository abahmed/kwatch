package transport

import (
	"errors"
	"net/http"
	"testing"
)

func TestPermanentWrapsAndDetects(t *testing.T) {
	if Permanent(nil) != nil {
		t.Fatal("Permanent(nil) must stay nil")
	}
	base := errors.New("bad request")
	err := Permanent(base)
	if !IsPermanent(err) || !errors.Is(err, base) {
		t.Fatalf("Permanent lost its cause: %v", err)
	}
	if err.Error() != "bad request" {
		t.Fatalf("message = %q", err.Error())
	}
	if IsPermanent(base) {
		t.Fatal("a plain error is not permanent")
	}
}

func TestRetryAfterErrorUnwraps(t *testing.T) {
	base := errors.New("slow down")
	err := &RetryAfterError{Err: base}
	if !errors.Is(err, base) || err.Error() != "slow down" {
		t.Fatalf("RetryAfterError = %v", err)
	}
}

func TestIsPermanentHTTPStatus(t *testing.T) {
	cases := map[int]bool{
		http.StatusOK: false, http.StatusBadRequest: true,
		http.StatusUnauthorized: true, http.StatusRequestTimeout: false,
		http.StatusTooManyRequests: false, http.StatusBadGateway: false,
	}
	for code, want := range cases {
		if got := IsPermanentHTTPStatus(code); got != want {
			t.Errorf("status %d: got %v, want %v", code, got, want)
		}
	}
}
