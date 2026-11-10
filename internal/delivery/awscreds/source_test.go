package awscreds

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/delivery/signing"
)

// countingFetch hands out key "K<n>" valid for ttl on its n-th call.
type countingFetch struct {
	clock *fakeClock
	ttl   time.Duration
	calls int
	err   error
}

func (f *countingFetch) fetch(
	context.Context,
) (signing.Credentials, time.Time, error) {
	f.calls++
	if f.err != nil {
		return signing.Credentials{}, time.Time{}, f.err
	}
	key := "K" + string(rune('0'+f.calls))
	return signing.Credentials{AccessKeyID: key},
		f.clock.Now().Add(f.ttl), nil
}

func keyOf(t *testing.T, src Source) string {
	t.Helper()
	creds, err := src.Credentials(context.Background())
	if err != nil {
		t.Fatalf("credentials: %v", err)
	}
	return creds.AccessKeyID
}

func TestCachedRefreshesFiveMinutesBeforeExpiry(t *testing.T) {
	clk := newFakeClock()
	f := &countingFetch{clock: clk, ttl: 15 * time.Minute}
	src := newCached("test", clk.Now, f.fetch)
	if keyOf(t, src) != "K1" || keyOf(t, src) != "K1" {
		t.Fatal("credentials were not cached")
	}
	clk.advance(9*time.Minute + 59*time.Second)
	if keyOf(t, src) != "K1" {
		t.Fatal("fetched again more than 5 minutes before expiry")
	}
	clk.advance(2 * time.Second)
	if got := keyOf(t, src); got != "K2" || f.calls != 2 {
		t.Fatalf("key = %s after %d fetches, want K2 after 2", got,
			f.calls)
	}
}

func TestCachedKeepsUnexpiredCredentialsWhenAFetchFails(t *testing.T) {
	clk := newFakeClock()
	f := &countingFetch{clock: clk, ttl: 15 * time.Minute}
	src := newCached("test", clk.Now, f.fetch)
	keyOf(t, src)
	f.err = errors.New("sts down")
	clk.advance(12 * time.Minute)
	if keyOf(t, src) != "K1" {
		t.Fatal("unexpired credentials were dropped")
	}
	clk.advance(4 * time.Minute)
	if _, err := src.Credentials(context.Background()); err == nil {
		t.Fatal("expired credentials were served")
	}
}

func TestCachedInvalidateFetchesAgain(t *testing.T) {
	clk := newFakeClock()
	f := &countingFetch{clock: clk, ttl: time.Hour}
	src := newCached("test", clk.Now, f.fetch)
	keyOf(t, src)
	src.Invalidate()
	if got := keyOf(t, src); got != "K2" {
		t.Fatalf("key after Invalidate = %s, want K2", got)
	}
}

func TestCachedRefusesCredentialsWithoutExpiry(t *testing.T) {
	src := newCached("test", newFakeClock().Now, func(
		context.Context) (signing.Credentials, time.Time, error) {
		return signing.Credentials{AccessKeyID: "K"}, time.Time{}, nil
	})
	if _, err := src.Credentials(context.Background()); err == nil {
		t.Fatal("credentials without an expiry were accepted")
	}
}

func TestRefreshable(t *testing.T) {
	if Refreshable(Static{}) || Refreshable(&Static{}) {
		t.Fatal("static keys are refreshable")
	}
	if !Refreshable(newCached("t", newFakeClock().Now, nil)) {
		t.Fatal("a cached source is not refreshable")
	}
}
