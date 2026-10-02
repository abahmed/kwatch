package delivery

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDeliveredHookFiresOnlyAfterProviderAccepts(t *testing.T) {
	provider := &errorRecorderProvider{name: "Primary"}
	signals := 0
	am := newTestManager()
	am.onDelivered = func() { signals++ }
	setManagerEntries(am, []providerEntry{{
		provider: provider,
		retry:    retryConfig{maxAttempts: 1, delay: time.Millisecond},
	}})
	entries := managerEntries(am)
	job := deliverJob{kind: jobMessage, msg: "hello"}

	am.deliverOne(context.Background(), &entries[0], job)
	if signals != 1 {
		t.Fatalf("signals after success = %d, want 1", signals)
	}

	provider.err = errors.New("provider down")
	am.deliverOne(context.Background(), &entries[0], job)
	if signals != 1 {
		t.Fatalf("signals after failure = %d, want 1", signals)
	}
}

func TestDeliveredHookIsOptional(t *testing.T) {
	am := newTestManager()
	setManagerEntries(am, []providerEntry{{
		provider: &errorRecorderProvider{name: "Primary"},
		retry:    retryConfig{maxAttempts: 1, delay: time.Millisecond},
	}})
	entries := managerEntries(am)
	if !am.deliverOne(context.Background(), &entries[0],
		deliverJob{kind: jobMessage, msg: "hello"}) {
		t.Fatal("delivery without a hook must still succeed")
	}
}
