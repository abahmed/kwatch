package opsgenie

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
	"github.com/abahmed/kwatch/internal/delivery/transport"
)

func noWait() func(context.Context, time.Duration) error {
	return func(context.Context, time.Duration) error { return nil }
}

// Opsgenie creates alerts asynchronously: an update right after the create
// can 404 once and must then succeed.
func TestOpsgenieUpdateRetriesWhileAlertIsBeingCreated(t *testing.T) {
	c, rec := newTestOpsgenie(t)
	var waits []time.Duration
	c.wait = func(_ context.Context, d time.Duration) error {
		waits = append(waits, d)
		return nil
	}
	puts := 0
	rec.Reply = func(w http.ResponseWriter, r providertest.Request) {
		if r.Method == "PUT" {
			puts++
			if puts == 1 {
				w.WriteHeader(http.StatusNotFound)
			}
		}
	}
	if err := c.SendIncident(
		context.Background(), providertest.Update()); err != nil {
		t.Fatalf("SendIncident: %v", err)
	}
	// POST create + failed priority + 3 successful updates.
	if got := len(rec.Requests()); got != 5 {
		t.Fatalf("requests = %d, want 5", got)
	}
	if len(waits) != 1 || waits[0] != updateRetryDelay {
		t.Fatalf("waits = %v", waits)
	}
}

// A 404 that never clears is bounded and comes back retryable, not
// permanent, so delivery tries the whole update again later.
func TestOpsgenieUpdateStaysBoundedAndRetryableOn404(t *testing.T) {
	c, rec := newTestOpsgenie(t)
	c.wait = noWait()
	rec.Reply = func(w http.ResponseWriter, r providertest.Request) {
		if r.Method == "PUT" {
			w.WriteHeader(http.StatusNotFound)
		}
	}
	err := c.SendIncident(context.Background(), providertest.Update())
	if err == nil {
		t.Fatal("want an error")
	}
	if transport.IsPermanent(err) {
		t.Fatalf("a 404 right after create must be retryable: %v", err)
	}
	if got := len(rec.Requests()); got != 1+updateAttempts {
		t.Fatalf("requests = %d, want create + %d tries",
			got, updateAttempts)
	}
}

func TestOpsgenieOtherUpdateErrorsAreNotRetried(t *testing.T) {
	c, rec := newTestOpsgenie(t)
	c.wait = noWait()
	rec.Reply = func(w http.ResponseWriter, r providertest.Request) {
		if r.Method == "PUT" {
			w.WriteHeader(http.StatusForbidden)
		}
	}
	err := c.SendIncident(context.Background(), providertest.Update())
	if err == nil || !transport.IsPermanent(err) {
		t.Fatalf("a 403 stays permanent, got %v", err)
	}
	if got := len(rec.Requests()); got != 2 {
		t.Fatalf("requests = %d, want create + one try", got)
	}
}
