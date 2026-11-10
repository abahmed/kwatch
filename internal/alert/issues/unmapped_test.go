package issues

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
	"github.com/abahmed/kwatch/internal/notification"
)

// markerTracker finds an issue only by the markers it is told about, as
// a real tracker does.
type markerTracker struct {
	searchTracker
	byMarker  map[string]string
	createErr error
}

func (s *markerTracker) Find(
	_ context.Context, marker string,
) (string, error) {
	s.searched = append(s.searched, marker)
	return s.byMarker[marker], nil
}

func (s *markerTracker) Create(
	ctx context.Context, title, body string,
) (string, error) {
	if s.createErr != nil {
		return "", s.createErr
	}
	return s.searchTracker.Create(ctx, title, body)
}

func keyed(msg notification.Message) notification.Message {
	msg.DedupKey = "alert-3f9a0c27d41e8b65"
	return msg
}

// A resolve whose mapping was lost (state saved late, evicted, reset)
// still closes the issue: the tracker is searched for it.
func TestMapUnmappedResolveClosesTheIssueItFinds(t *testing.T) {
	m := NewMap(time.Now)
	resolve := providertest.Resolve()
	tracker := &markerTracker{byMarker: map[string]string{
		Marker(resolve.ThreadKey()): "17",
	}}

	err := m.Deliver(context.Background(), tracker, resolve, "t", "b")

	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(tracker.calls, ","); got != "close:17" {
		t.Fatalf("calls = %q, want the found issue closed", got)
	}
	if m.HasThread(resolve.ThreadKey()) {
		t.Fatal("a finished incident must not be mapped")
	}
}

// Nothing found means nothing to close, and a failed search is retried.
func TestMapUnmappedResolveWithNothingFound(t *testing.T) {
	m := NewMap(time.Now)
	tracker := &searchTracker{}
	err := m.Deliver(context.Background(), tracker,
		providertest.Resolve(), "t", "b")
	if err != nil || len(tracker.calls) != 0 {
		t.Fatalf("err = %v, calls = %v", err, tracker.calls)
	}
	tracker.findErr = errors.New("timeout")
	err = m.Deliver(context.Background(), tracker,
		providertest.Resolve(), "t", "b")
	if err == nil {
		t.Fatal("a resolve whose search failed must be retried")
	}
}

// An update that finds its issue by marker is posted as a comment: the
// issue is adopted and the text is not lost.
func TestMapUnmappedUpdateCommentsOnTheIssueItFinds(t *testing.T) {
	m := NewMap(time.Now)
	update := providertest.Update()
	tracker := &markerTracker{byMarker: map[string]string{
		Marker(update.ThreadKey()): "17",
	}}

	err := m.Deliver(context.Background(), tracker, update, "t", "b")

	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(tracker.calls, ","); got != "comment:17" {
		t.Fatalf("calls = %q, want one comment", got)
	}
	if id, _ := m.lookup(update.ThreadKey()); id != "17" {
		t.Fatalf("mapped to %q, want the found issue", id)
	}
}

// An issue found after this process's own create failed is that create:
// it already carries the text, so nothing is commented.
func TestMapRetriedCreateDoesNotCommentItsOwnIssue(t *testing.T) {
	m := NewMap(time.Now)
	update := providertest.Update()
	tracker := &markerTracker{createErr: errors.New("timeout")}
	ctx := context.Background()
	if err := m.Deliver(ctx, tracker, update, "t", "b"); err == nil {
		t.Fatal("the failed create must be reported")
	}
	tracker.byMarker = map[string]string{Marker(update.ThreadKey()): "17"}

	if err := m.Deliver(ctx, tracker, update, "t", "b"); err != nil {
		t.Fatal(err)
	}
	if len(tracker.calls) != 0 {
		t.Fatalf("calls = %v, want none", tracker.calls)
	}
}

// A new issue carries the marker made of the alert key and the cluster.
// That key survives a reset of kwatch's state, so the issue is found
// under a new conversation key, and another cluster's issue is not.
func TestMapMarkerSurvivesAStateReset(t *testing.T) {
	ctx := context.Background()
	before := NewMap(time.Now).ForCluster("east")
	tracker := &markerTracker{byMarker: map[string]string{}}
	announce := keyed(providertest.Announce())
	if err := before.Deliver(ctx, tracker, announce, "t", "b"); err != nil {
		t.Fatal(err)
	}
	stable := Marker(announce.AlertKey("east"))
	if !strings.HasSuffix(tracker.bodies[0], "Ref: "+stable) {
		t.Fatalf("create body = %q, want the stable marker", tracker.bodies)
	}
	tracker.byMarker[stable] = "42"
	tracker.calls = nil

	resolve := keyed(providertest.Resolve())
	resolve.Key = "inc-after-reset"
	after := NewMap(time.Now).ForCluster("east")
	if err := after.Deliver(ctx, tracker, resolve, "t", "b"); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(tracker.calls, ","); got != "close:42" {
		t.Fatalf("calls = %q, want the old issue closed", got)
	}

	tracker.calls = nil
	other := NewMap(time.Now).ForCluster("west")
	if err := other.Deliver(ctx, tracker, resolve, "t", "b"); err != nil {
		t.Fatal(err)
	}
	if len(tracker.calls) != 0 {
		t.Fatalf("another cluster closed the issue: %v", tracker.calls)
	}
}

// Issues filed before the stable marker carry only the marker of their
// conversation key; they are still found.
func TestMapStillFindsIssuesWithTheOldMarker(t *testing.T) {
	m := NewMap(time.Now).ForCluster("east")
	resolve := keyed(providertest.Resolve())
	tracker := &markerTracker{byMarker: map[string]string{
		Marker(resolve.ThreadKey()): "9",
	}}

	err := m.Deliver(context.Background(), tracker, resolve, "t", "b")

	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(tracker.calls, ","); got != "close:9" {
		t.Fatalf("calls = %q, want the old-marker issue closed", got)
	}
	if len(tracker.searched) != 2 {
		t.Fatalf("searched %v, want the new marker then the old", tracker.searched)
	}
}
