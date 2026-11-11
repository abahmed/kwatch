package issues

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

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
	m := NewMap(time.Now)
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
	m := NewMap(time.Now)
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

type splitCloseTracker struct {
	fakeTracker
	stateErrs int
	states    int
}

func (s *splitCloseTracker) CloseState(_ context.Context, id string) error {
	s.calls = append(s.calls, "state:"+id)
	if s.stateErrs > 0 {
		s.stateErrs--
		return errors.New("status code 500: boom")
	}
	s.states++
	return nil
}

// A state change that fails after the resolve comment was posted is
// retried alone: the comment must not be posted a second time.
func TestResolveRetryDoesNotRepeatTheComment(t *testing.T) {
	m := NewMap(time.Now)
	tr := &splitCloseTracker{stateErrs: 1}
	openIssue(t, m, tr)
	ctx := context.Background()
	if err := m.Deliver(ctx, tr, providertest.Resolve(),
		"t", "b"); err == nil {
		t.Fatal("want the state failure back")
	}
	if err := m.Deliver(ctx, tr, providertest.Resolve(),
		"t", "b"); err != nil {
		t.Fatal(err)
	}
	want := "create:t,comment:42,state:42,state:42"
	if got := strings.Join(tr.calls, ","); got != want {
		t.Fatalf("calls = %s, want %s", got, want)
	}
	if len(m.SnapshotThreads()) != 0 || len(m.commented) != 0 {
		t.Fatal("resolve left state behind")
	}
}
