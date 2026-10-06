package issues

import (
	"context"
	"errors"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
	"github.com/abahmed/kwatch/internal/delivery/transport"
)

type closeFailTracker struct {
	fakeTracker
	closeErr error
}

func (c *closeFailTracker) Close(
	ctx context.Context, id, body string,
) error {
	_ = c.fakeTracker.Close(ctx, id, body)
	return c.closeErr
}

func openIssue(t *testing.T, m *Map, tr Tracker) {
	t.Helper()
	err := m.Deliver(context.Background(), tr, providertest.Announce(),
		"t", "b")
	if err != nil {
		t.Fatal(err)
	}
}

// Closing an issue that was deleted (404) must forget the mapping, so the
// resolve is not retried forever.
func TestCloseOn404ForgetsMapping(t *testing.T) {
	m := NewMap()
	tr := &closeFailTracker{closeErr: transport.Permanent(
		&transport.StatusError{Provider: "GitHub", StatusCode: 404,
			Body: "Not Found"})}
	openIssue(t, m, tr)
	err := m.Deliver(context.Background(), tr, providertest.Resolve(),
		"t", "b")
	if err != nil {
		t.Fatalf("404 on close is not an error: %v", err)
	}
	if len(m.SnapshotThreads()) != 0 {
		t.Fatal("mapping kept after the issue vanished")
	}
}

// Any other close failure keeps the mapping so the retry can close it.
func TestCloseOtherErrorKeepsMapping(t *testing.T) {
	m := NewMap()
	tr := &closeFailTracker{closeErr: errors.New(
		"call to GitHub returned status code 500: boom")}
	openIssue(t, m, tr)
	err := m.Deliver(context.Background(), tr, providertest.Resolve(),
		"t", "b")
	if err == nil {
		t.Fatal("want the error back")
	}
	if len(m.SnapshotThreads()) != 1 {
		t.Fatal("mapping dropped on a retryable failure")
	}
}
