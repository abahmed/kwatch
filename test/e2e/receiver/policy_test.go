package main

import (
	"net/http"
	"testing"
)

func TestPolicyResponse(t *testing.T) {
	tests := []struct {
		name   string
		mode   string
		status int
	}{
		{"success", "success", http.StatusOK},
		{"server error", "http-500", http.StatusInternalServerError},
		{"rate limited", "http-429", http.StatusTooManyRequests},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			status, _, err := (Policy{Mode: test.mode}).response()
			if err != nil {
				t.Fatal(err)
			}
			if status != test.status {
				t.Fatalf("got status %d, want %d", status, test.status)
			}
		})
	}
}

func TestPolicyConnectionReset(t *testing.T) {
	status, _, err := (Policy{Mode: "connection-reset"}).response()
	if err != nil {
		t.Fatal(err)
	}
	if status != 0 {
		t.Fatalf("status = %d, want zero", status)
	}
}
