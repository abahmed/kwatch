package issues

import (
	"context"
	"testing"

	"github.com/abahmed/kwatch/internal/event"
)

type fakeTracker struct{ calls []string }

func (f *fakeTracker) Create(
	_ context.Context, title, _ string,
) (string, error) {
	f.calls = append(f.calls, "create:"+title)
	return "42", nil
}

func (f *fakeTracker) Comment(_ context.Context, id, _ string) error {
	f.calls = append(f.calls, "comment:"+id)
	return nil
}

func (f *fakeTracker) Close(_ context.Context, id, _ string) error {
	f.calls = append(f.calls, "close:"+id)
	return nil
}

func TestMapFilesOneIssuePerIncident(t *testing.T) {
	m := NewMap()
	tracker := &fakeTracker{}
	ctx := context.Background()
	for _, action := range []string{"create", "update", "resolved"} {
		e := &event.Event{DedupKey: "abc", Action: action, Narrative: "x"}
		if err := m.Deliver(ctx, tracker, e, "t", "b"); err != nil {
			t.Fatal(err)
		}
	}
	notice := &event.Event{PodName: "started", Reason: "notify"}
	if err := m.Deliver(ctx, tracker, notice, "t", "b"); err != nil {
		t.Fatal(err)
	}
	want := []string{"create:t", "comment:42", "close:42"}
	if len(tracker.calls) != len(want) {
		t.Fatalf("calls = %v, want %v", tracker.calls, want)
	}
	for i := range want {
		if tracker.calls[i] != want[i] {
			t.Fatalf("calls = %v, want %v", tracker.calls, want)
		}
	}
	if len(m.SnapshotThreads()) != 0 {
		t.Fatal("closed issue still tracked")
	}
}

func TestMapRestoresIssuesAcrossRestart(t *testing.T) {
	m := NewMap()
	m.RestoreThreads(map[string]string{"kwatch-abc": "7"})
	tracker := &fakeTracker{}
	e := &event.Event{DedupKey: "abc", Action: "update", Narrative: "x"}
	if err := m.Deliver(context.Background(), tracker, e, "t", "b"); err != nil {
		t.Fatal(err)
	}
	if len(tracker.calls) != 1 || tracker.calls[0] != "comment:7" {
		t.Fatalf("calls = %v", tracker.calls)
	}
}
