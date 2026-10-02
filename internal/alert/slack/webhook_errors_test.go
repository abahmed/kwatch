package slack

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/ratelimit"
)

const webhookSecret = "T000/B000/SECRETxyz"

func newWebhookSlack(t *testing.T, url string) *Slack {
	t.Helper()
	s := newTestSlack(map[string]interface{}{"webhook": url}, "dev")
	if s == nil {
		t.Fatal("webhook slack not constructed")
	}
	return s
}

func assertNoWebhookSecret(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error")
	}
	if strings.Contains(err.Error(), "SECRETxyz") {
		t.Fatalf("error leaks webhook URL: %v", err)
	}
}

func TestSlackWebhookErrorClassification(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		permanent bool
		limited   bool
	}{
		{name: "not found is permanent", status: 404, permanent: true},
		{name: "server error is retryable", status: 502},
		{name: "rate limit is limited", status: 429, limited: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(
				func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(tc.status)
				}))
			defer srv.Close()

			s := newWebhookSlack(t, srv.URL+"/services/"+webhookSecret)
			err := s.SendMessage(context.Background(), "hello")

			assertNoWebhookSecret(t, err)
			if got := transport.IsPermanent(err); got != tc.permanent {
				t.Fatalf("IsPermanent = %v, want %v (%v)",
					got, tc.permanent, err)
			}
			var limited *ratelimit.Error
			if got := errors.As(err, &limited); got != tc.limited {
				t.Fatalf("rate limited = %v, want %v (%v)",
					got, tc.limited, err)
			}
		})
	}
}

func TestSlackWebhookNetworkErrorHidesURL(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL + "/services/" + webhookSecret
	srv.Close()

	err := newWebhookSlack(t, url).
		SendMessage(context.Background(), "hello")
	assertNoWebhookSecret(t, err)
}

func TestSlackWebhookInvalidURLHidesSecret(t *testing.T) {
	url := "https://hooks.slack.com/services/" + webhookSecret + "\n"
	err := newWebhookSlack(t, url).
		SendMessage(context.Background(), "hello")
	assertNoWebhookSecret(t, err)
}
