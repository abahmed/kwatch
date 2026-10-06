package issues

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
	"github.com/abahmed/kwatch/internal/delivery/transport"
	"github.com/abahmed/kwatch/internal/notification"
)

// reopenTracker is a tracker that can also reopen.
type reopenTracker struct {
	fakeTracker
}

func (r *reopenTracker) Reopen(_ context.Context, id string) error {
	r.calls = append(r.calls, "reopen:"+id)
	return nil
}

func resolveWithWindow(window time.Duration) notification.Message {
	m := providertest.Resolve()
	m.ReopenWithin = window
	return m
}

func deliver(t *testing.T, m *Map, tr Tracker, msg notification.Message) {
	t.Helper()
	if err := m.Deliver(context.Background(), tr, msg, "t", "b"); err != nil {
		t.Fatal(err)
	}
}

func TestReopenInsideWindowCommentsOnTheSameIssue(t *testing.T) {
	now := time.Unix(1000, 0)
	m := NewMap()
	m.now = func() time.Time { return now }
	tr := &reopenTracker{}

	deliver(t, m, tr, providertest.Announce())
	deliver(t, m, tr, resolveWithWindow(10*time.Minute))
	now = now.Add(5 * time.Minute)
	deliver(t, m, tr, providertest.Update())

	want := "create:t,close:42,reopen:42,comment:42"
	if got := strings.Join(tr.calls, ","); got != want {
		t.Fatalf("calls = %s, want %s", got, want)
	}
	// Open again: the next resolve closes it once more.
	deliver(t, m, tr, resolveWithWindow(10*time.Minute))
	if got := tr.calls[len(tr.calls)-1]; got != "close:42" {
		t.Fatalf("last call = %s, want close:42", got)
	}
}

func TestReopenAfterWindowOpensANewIssue(t *testing.T) {
	now := time.Unix(1000, 0)
	m := NewMap()
	m.now = func() time.Time { return now }
	tr := &fakeTracker{}

	deliver(t, m, tr, providertest.Announce())
	deliver(t, m, tr, resolveWithWindow(10*time.Minute))
	now = now.Add(11 * time.Minute)
	deliver(t, m, tr, providertest.Update())

	want := "create:t,close:42,create:t"
	if got := strings.Join(tr.calls, ","); got != want {
		t.Fatalf("calls = %s, want %s", got, want)
	}
}

func TestTrackerThatCannotReopenForgetsAtResolve(t *testing.T) {
	m := NewMap()
	tr := &fakeTracker{}
	deliver(t, m, tr, providertest.Announce())
	deliver(t, m, tr, resolveWithWindow(time.Hour))
	deliver(t, m, tr, providertest.Update())
	// A new issue, not a comment on the closed one.
	want := "create:t,close:42,create:t"
	if got := strings.Join(tr.calls, ","); got != want {
		t.Fatalf("calls = %s, want %s", got, want)
	}
}

// conditionalTracker reopens only when it says so.
type conditionalTracker struct {
	reopenTracker
	can bool
}

func (c *conditionalTracker) CanReopen() bool { return c.can }

func TestUnconfiguredReopenerForgetsAtResolve(t *testing.T) {
	for _, can := range []bool{false, true} {
		m := NewMap()
		tr := &conditionalTracker{can: can}
		deliver(t, m, tr, providertest.Announce())
		deliver(t, m, tr, resolveWithWindow(time.Hour))
		deliver(t, m, tr, providertest.Update())
		got := tr.calls[len(tr.calls)-1]
		if can && got != "comment:42" || !can && got != "create:t" {
			t.Fatalf("can=%v: last call = %s", can, got)
		}
	}
}

func TestResolveWithoutWindowForgetsAtOnce(t *testing.T) {
	m := NewMap()
	tr := &fakeTracker{}
	deliver(t, m, tr, providertest.Announce())
	deliver(t, m, tr, providertest.Resolve())
	deliver(t, m, tr, providertest.Update())
	if got := strings.Join(tr.calls, ","); got != "create:t,close:42,create:t" {
		t.Fatalf("calls = %s", got)
	}
}

func TestDuplicateResolveDoesNotCloseTwice(t *testing.T) {
	m := NewMap()
	tr := &fakeTracker{}
	deliver(t, m, tr, providertest.Announce())
	deliver(t, m, tr, resolveWithWindow(time.Hour))
	deliver(t, m, tr, resolveWithWindow(time.Hour))
	if got := strings.Join(tr.calls, ","); got != "create:t,close:42" {
		t.Fatalf("calls = %s", got)
	}
}

func TestClosedIssueSurvivesARestartUntilTheWindowEnds(t *testing.T) {
	now := time.Unix(1000, 0)
	m := NewMap()
	m.now = func() time.Time { return now }
	tr := &reopenTracker{}
	deliver(t, m, tr, providertest.Announce())
	deliver(t, m, tr, resolveWithWindow(10*time.Minute))
	saved := m.SnapshotThreads()

	restored := NewMap()
	restored.now = func() time.Time { return now.Add(time.Minute) }
	restored.RestoreThreads(saved)
	deliver(t, restored, tr, providertest.Update())
	if got := tr.calls[len(tr.calls)-1]; got != "comment:42" {
		t.Fatalf("last call = %s, want a comment on the old issue", got)
	}

	late := NewMap()
	late.now = func() time.Time { return now.Add(time.Hour) }
	late.RestoreThreads(saved)
	if len(late.SnapshotThreads()) != 0 {
		t.Fatal("an expired window was restored")
	}
}

// vanishingTracker answers the first comment with a 404, as GitHub does
// for a deleted issue, and works afterwards.
type vanishingTracker struct {
	fakeTracker
	vanished bool
}

func (v *vanishingTracker) Comment(
	ctx context.Context, id, body string,
) error {
	if !v.vanished {
		v.vanished = true
		v.calls = append(v.calls, "comment-404:"+id)
		return transport.Permanent(
			&transport.StatusError{Provider: "x", StatusCode: 404})
	}
	return v.fakeTracker.Comment(ctx, id, body)
}

func TestCommentOnDeletedIssueOpensANewOne(t *testing.T) {
	m := NewMap()
	tr := &vanishingTracker{}
	deliver(t, m, tr, providertest.Announce())
	deliver(t, m, tr, providertest.Update())
	want := "create:t,comment-404:42,create:t"
	if got := strings.Join(tr.calls, ","); got != want {
		t.Fatalf("calls = %s, want %s", got, want)
	}
}

// openTracker says whether its Close really ends the issue, like Jira and
// ClickUp without a close setting.
type openTracker struct {
	reopenTracker
	closes bool
}

func (o *openTracker) ClosesIssues() bool { return o.closes }
func (o *openTracker) CanReopen() bool    { return o.closes }

// A resolve that only commented leaves the issue open, so a recurrence
// comments on it. A tracker that closed it and can reopen reopens it. Either
// way no second issue is created.
func TestUnclosedIssueKeepsItsMappingAtResolve(t *testing.T) {
	for _, tc := range []struct {
		closes bool
		want   string
	}{
		{false, "create:t,close:42,comment:42"},
		{true, "create:t,close:42,reopen:42,comment:42"},
	} {
		m := NewMap()
		tr := &openTracker{closes: tc.closes}
		deliver(t, m, tr, providertest.Announce())
		deliver(t, m, tr, resolveWithWindow(time.Hour))
		deliver(t, m, tr, providertest.Update())
		if got := strings.Join(tr.calls, ","); got != tc.want {
			t.Fatalf("closes=%v: calls = %s, want %s",
				tc.closes, got, tc.want)
		}
	}
}
