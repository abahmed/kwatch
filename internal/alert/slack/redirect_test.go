package slack

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/transport"
)

// A webhook URL that redirects (a proxy, the wrong scheme) is never
// followed, so retrying it cannot work: it is permanent, with the same
// message the shared transport gives, and the fallback takes over.
func TestSlackWebhookRedirectIsPermanent(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			http.Redirect(w, r, "/elsewhere", http.StatusFound)
		}))
	defer srv.Close()
	s := newWebhookSlack(t, srv.URL+"/services/"+webhookSecret)

	err := s.SendMessage(context.Background(), "hello")

	assertNoWebhookSecret(t, err)
	if !transport.IsPermanent(err) {
		t.Fatalf("a redirect must be permanent: %v", err)
	}
	if !strings.Contains(err.Error(), "check the configured URL") {
		t.Fatalf("error = %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("requests = %d, the redirect was followed", calls.Load())
	}
}
