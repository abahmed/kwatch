package issues

import (
	"context"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
	"github.com/abahmed/kwatch/internal/metrics"
	"github.com/abahmed/kwatch/internal/notification"
)

func TestMapSkipsUpdatesForUntrackedIssue(t *testing.T) {
	m := NewMap()
	tracker := &fakeTracker{unreadable: true}
	ctx := context.Background()
	counter := &metrics.DefaultRegistry().Delivery.TrackerUntracked
	before := counter.Load()
	for _, tc := range providertest.Lifecycle() {
		if err := m.Deliver(ctx, tracker, tc.Message, "t", "b"); err != nil {
			t.Fatalf("%s: %v", tc.Name, err)
		}
		if len(m.SnapshotThreads()) != 0 {
			t.Fatalf("%s: untracked marker was snapshotted", tc.Name)
		}
	}
	if got := strings.Join(tracker.calls, ","); got != "create:t" {
		t.Fatalf("calls = %v, want one create", got)
	}
	if got := counter.Load() - before; got != 1 {
		t.Fatalf("untracked counter grew by %d, want 1", got)
	}
	if m.lookup(providertest.Announce().ThreadKey()) != "" {
		t.Fatal("resolve kept the untracked marker")
	}
}

func TestMapOpensNewIssueAfterUntrackedResolve(t *testing.T) {
	m := NewMap()
	tracker := &fakeTracker{unreadable: true}
	ctx := context.Background()
	for _, msg := range []notification.Message{
		providertest.Announce(), providertest.Resolve(),
		providertest.Announce(),
	} {
		if err := m.Deliver(ctx, tracker, msg, "t", "b"); err != nil {
			t.Fatal(err)
		}
	}
	if got := strings.Join(tracker.calls, ","); got != "create:t,create:t" {
		t.Fatalf("calls = %v", got)
	}
}

func TestMapRestoreIgnoresUntrackedMarker(t *testing.T) {
	m := NewMap()
	m.RestoreThreads(map[string]string{
		providertest.Announce().ThreadKey(): untracked,
	})
	if m.lookup(providertest.Announce().ThreadKey()) != "" {
		t.Fatal("restore accepted the untracked marker")
	}
}
