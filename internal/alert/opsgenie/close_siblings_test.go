package opsgenie

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
)

// A reconfiguration leaves the old instance running. Its waiting close
// must not close the alert the new instance opens for the same alias.
func TestOpsgenieNewGenerationCancelsTheOldOnesPendingClose(t *testing.T) {
	old, rec := newTestOpsgenie(t)
	release := make(chan struct{})
	old.wait = func(ctx context.Context, _ time.Duration) error {
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	rec.Reply = func(w http.ResponseWriter, r providertest.Request) {
		if len(r.Path) > 6 && r.Path[len(r.Path)-6:] == "/close" {
			w.WriteHeader(http.StatusNotFound)
		}
	}
	_ = old.SendIncident(context.Background(), providertest.Resolve())
	next := NewOpsgenie(map[string]interface{}{"apiKey": "secret"}, "dev",
		rec.Dependencies())
	next.url = old.url

	if err := next.SendIncident(
		context.Background(), providertest.Announce()); err != nil {
		t.Fatalf("SendIncident: %v", err)
	}
	close(release)
	settle(old)

	if got := closeRequests(rec); got != 1 {
		t.Fatalf("close requests = %d, want only the first", got)
	}
}
