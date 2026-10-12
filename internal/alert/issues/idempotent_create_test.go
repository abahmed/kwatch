package issues

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
	"github.com/abahmed/kwatch/internal/delivery/transport"
)

// searchTracker is a fakeTracker that can search by marker and records the
// body of its create.
type searchTracker struct {
	fakeTracker
	found    string
	findErr  error
	bodies   []string
	searched []string
}

func (s *searchTracker) Find(
	_ context.Context, marker string,
) (string, error) {
	s.searched = append(s.searched, marker)
	return s.found, s.findErr
}

func (s *searchTracker) Create(
	ctx context.Context, title, body string,
) (string, error) {
	s.bodies = append(s.bodies, body)
	return s.fakeTracker.Create(ctx, title, body)
}

func TestMapCreateAdoptsIssueFoundByMarker(t *testing.T) {
	m := NewMap(time.Now)
	tracker := &searchTracker{found: "17"}

	err := m.Deliver(context.Background(), tracker,
		providertest.Announce(), "t", "b")

	if err != nil {
		t.Fatal(err)
	}
	if len(tracker.calls) != 0 {
		t.Fatalf("a found issue must not be created again: %v", tracker.calls)
	}
	key := providertest.Announce().ThreadKey()
	if want := Marker(key); tracker.searched[0] != want {
		t.Fatalf("searched %v, want %s", tracker.searched, want)
	}
	if id, _ := m.lookup(key); id != "17" {
		t.Fatalf("mapped to %q, want the found issue", id)
	}
}

func TestMapCreateEmbedsMarkerWhenNothingFound(t *testing.T) {
	m := NewMap(time.Now)
	tracker := &searchTracker{}

	err := m.Deliver(context.Background(), tracker,
		providertest.Announce(), "t", "b")

	if err != nil {
		t.Fatal(err)
	}
	marker := Marker(providertest.Announce().ThreadKey())
	if len(tracker.bodies) != 1 ||
		!strings.HasSuffix(tracker.bodies[0], "Ref: "+marker) {
		t.Fatalf("create body = %q", tracker.bodies)
	}
}

func TestMapCreateRetriesAfterTransientSearchFailure(t *testing.T) {
	m := NewMap(time.Now)
	tracker := &searchTracker{findErr: errors.New("timeout")}

	err := m.Deliver(context.Background(), tracker,
		providertest.Announce(), "t", "b")

	if err == nil || transport.IsPermanent(err) {
		t.Fatalf("a failed search must be retryable: %v", err)
	}
	if len(tracker.calls) != 0 {
		t.Fatal("must not create while the search outcome is unknown")
	}
}

func TestMapCreateFallsBackWhenSearchIsRefused(t *testing.T) {
	m := NewMap(time.Now)
	tracker := &searchTracker{
		findErr: transport.Permanent(errors.New("forbidden")),
	}

	err := m.Deliver(context.Background(), tracker,
		providertest.Announce(), "t", "b")

	if err != nil {
		t.Fatal(err)
	}
	if len(tracker.calls) != 1 {
		t.Fatalf("calls = %v, want one create", tracker.calls)
	}
}

func TestMapEvictionPrefersClosedIssues(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	m := NewMap(func() time.Time { return now })
	for i := 0; i < maxTrackedIssues; i++ {
		m.remember(fmt.Sprintf("open-%04d", i), "1")
	}
	m.mu.Lock()
	m.closedUntil["open-0500"] = now.Add(time.Hour)
	m.mu.Unlock()

	m.remember("new", "2")

	if id, _ := m.lookup("open-0500"); id != "" {
		t.Fatal("the closed issue should have been evicted")
	}
	if id, _ := m.lookup("open-0000"); id == "" {
		t.Fatal("an open issue was evicted while a closed one existed")
	}
	if len(m.ids) != maxTrackedIssues {
		t.Fatalf("size = %d", len(m.ids))
	}
}

func TestMapEvictionDropsOldestOpenWhenNoneClosed(t *testing.T) {
	m := NewMap(time.Now)
	for i := 0; i < maxTrackedIssues+1; i++ {
		m.remember(fmt.Sprintf("open-%04d", i), "1")
	}

	if id, _ := m.lookup("open-0000"); id != "" {
		t.Fatal("the oldest open issue should go when nothing is closed")
	}
}

func TestMapUsesInjectedClockForReopenWindow(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	m := NewMap(func() time.Time { return now })
	m.remember("k", "1")

	m.finish("k", time.Hour)
	if _, closed := m.lookup("k"); !closed {
		t.Fatal("issue should be closed inside its window")
	}
	now = now.Add(time.Hour)
	if id, _ := m.lookup("k"); id != "" {
		t.Fatal("window end must be read from the injected clock")
	}
}
