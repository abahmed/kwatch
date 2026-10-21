package opsgenie

import (
	"context"
	"net/http"
	"sync/atomic"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
	"github.com/abahmed/kwatch/internal/metrics"
)

// settle waits until no close is retried in the background any more.
func settle(c *Opsgenie) { c.closes.running.Wait() }

func closeRequests(rec *providertest.Recorder) int {
	n := 0
	for _, req := range rec.Requests() {
		if req.Method == "POST" && len(req.Path) > 6 &&
			req.Path[len(req.Path)-6:] == "/close" {
			n++
		}
	}
	return n
}

func lostResolves() int64 {
	return metrics.DefaultRegistry().DeliveryResolvesLost.Load()
}

// An alert that Opsgenie takes longer than a few seconds to create is
// still closed: the close is retried in the background with growing
// waits, and the caller (the provider's queue) is not held for it.
func TestOpsgenieCloseOfSlowAlertIsRetriedInTheBackground(t *testing.T) {
	c, rec := newTestOpsgenie(t)
	var waits []time.Duration
	c.wait = func(_ context.Context, d time.Duration) error {
		waits = append(waits, d)
		return nil
	}
	var calls atomic.Int32
	rec.Reply = func(w http.ResponseWriter, _ providertest.Request) {
		if calls.Add(1) <= 6 {
			w.WriteHeader(http.StatusNotFound)
		}
	}
	before := lostResolves()

	if err := c.SendIncident(
		context.Background(), providertest.Resolve()); err != nil {
		t.Fatalf("a close that is not visible yet is not an error: %v", err)
	}
	settle(c)

	if got := closeRequests(rec); got != 7 {
		t.Fatalf("close requests = %d, want 6 misses and 1 hit", got)
	}
	if len(waits) != 6 || waits[0] >= waits[1] || waits[1] >= waits[2] {
		t.Fatalf("waits must grow: %v", waits)
	}
	if lostResolves() != before {
		t.Fatal("a close that went through is not a lost resolve")
	}
}

// The wait of a pending close never holds the caller.
func TestOpsgenieCloseDoesNotHoldTheQueueWhileItWaits(t *testing.T) {
	c, rec := newTestOpsgenie(t)
	release := make(chan struct{})
	c.wait = func(ctx context.Context, _ time.Duration) error {
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	var calls atomic.Int32
	rec.Reply = func(w http.ResponseWriter, _ providertest.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusNotFound)
		}
	}

	done := make(chan error, 1)
	go func() {
		done <- c.SendIncident(context.Background(), providertest.Resolve())
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("SendIncident: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("SendIncident waits for the pending close")
	}
	close(release)
	settle(c)
	if got := closeRequests(rec); got != 2 {
		t.Fatalf("close requests = %d, want 2", got)
	}
}

// A close that never finds its alert is given up after the old grace of
// five minutes, counted and logged, never retried for ever.
func TestOpsgenieCloseGivesUpAfterTheGraceAndCountsIt(t *testing.T) {
	c, rec := newTestOpsgenie(t)
	var total time.Duration
	c.wait = func(_ context.Context, d time.Duration) error {
		total += d
		return nil
	}
	rec.Reply = func(w http.ResponseWriter, _ providertest.Request) {
		w.WriteHeader(http.StatusNotFound)
	}
	before := lostResolves()

	if err := c.SendIncident(
		context.Background(), providertest.Resolve()); err != nil {
		t.Fatalf("a close that is given up is not an error: %v", err)
	}
	settle(c)

	if total != closeRetryWindow {
		t.Fatalf("waited %v in total, want %v", total, closeRetryWindow)
	}
	if got := lostResolves() - before; got != 1 {
		t.Fatalf("lost resolves counted = %d, want 1", got)
	}
	attempts := closeRequests(rec)
	if attempts < 5 || attempts > 20 {
		t.Fatalf("close requests = %d, want a bounded handful", attempts)
	}
}

// A rejection (a wrong key) is not waited out.
func TestOpsgenieBackgroundCloseStopsOnARejection(t *testing.T) {
	c, rec := newTestOpsgenie(t)
	c.wait = noWait()
	var calls atomic.Int32
	rec.Reply = func(w http.ResponseWriter, _ providertest.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusForbidden)
	}
	before := lostResolves()

	_ = c.SendIncident(context.Background(), providertest.Resolve())
	settle(c)

	if got := closeRequests(rec); got != 2 {
		t.Fatalf("close requests = %d, want 2", got)
	}
	if got := lostResolves() - before; got != 1 {
		t.Fatalf("lost resolves counted = %d, want 1", got)
	}
}

// A new alert for the same alias must not be closed by a close that is
// still waiting for the old one.
func TestOpsgenieCreateCancelsAPendingCloseOfTheSameAlias(t *testing.T) {
	c, rec := newTestOpsgenie(t)
	release := make(chan struct{})
	c.wait = func(ctx context.Context, _ time.Duration) error {
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
	_ = c.SendIncident(context.Background(), providertest.Resolve())
	if err := c.SendIncident(
		context.Background(), providertest.Announce()); err != nil {
		t.Fatalf("SendIncident: %v", err)
	}

	close(release)
	settle(c)

	if got := closeRequests(rec); got != 1 {
		t.Fatalf("close requests = %d, want only the first", got)
	}
}

// Shutdown ends the wait at once, and the close that never went through
// is counted.
func TestOpsgenieShutdownEndsAPendingClose(t *testing.T) {
	c, rec := newTestOpsgenie(t)
	c.wait = func(ctx context.Context, _ time.Duration) error {
		<-ctx.Done()
		return ctx.Err()
	}
	rec.Reply = func(w http.ResponseWriter, _ providertest.Request) {
		w.WriteHeader(http.StatusNotFound)
	}
	ctx, cancel := context.WithCancel(context.Background())
	before := lostResolves()
	_ = c.SendIncident(ctx, providertest.Resolve())

	cancel()
	settle(c)

	if got := lostResolves() - before; got != 1 {
		t.Fatalf("lost resolves counted = %d, want 1", got)
	}
}

// The number of closes kept for later is bounded.
func TestOpsgenieKeepsABoundedNumberOfPendingCloses(t *testing.T) {
	c, rec := newTestOpsgenie(t)
	release := make(chan struct{})
	c.wait = func(ctx context.Context, _ time.Duration) error {
		select {
		case <-release:
			return context.Canceled
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	rec.Reply = func(w http.ResponseWriter, _ providertest.Request) {
		w.WriteHeader(http.StatusNotFound)
	}
	before := lostResolves()
	for i := 0; i < maxPendingCloses+5; i++ {
		m := providertest.Resolve()
		m.Key = m.Key + "-" + string(rune('a'+i%26)) + string(rune('a'+i/26))
		_ = c.SendIncident(context.Background(), m)
	}
	if got := lostResolves() - before; got != 5 {
		t.Fatalf("closes given up at the bound = %d, want 5", got)
	}
	close(release)
	settle(c)
}
