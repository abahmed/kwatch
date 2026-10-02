package issues

import (
	"context"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/delivery/providertest"
	"github.com/abahmed/kwatch/internal/notification"
)

type fakeTracker struct {
	calls []string
	// unreadable makes Create succeed without an issue reference.
	unreadable bool
}

func (f *fakeTracker) Name() string { return "fake" }

func (f *fakeTracker) Create(
	_ context.Context, title, _ string,
) (string, error) {
	f.calls = append(f.calls, "create:"+title)
	if f.unreadable {
		return "", nil
	}
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
	for _, tc := range providertest.Lifecycle() {
		if err := m.Deliver(ctx, tracker, tc.Message, "t", "b"); err != nil {
			t.Fatal(err)
		}
	}
	notice := notification.Notice("kwatch started")
	if err := m.Deliver(ctx, tracker, notice, "t", "b"); err != nil {
		t.Fatal(err)
	}
	want := "create:t,comment:42,close:42"
	if got := strings.Join(tracker.calls, ","); got != want {
		t.Fatalf("calls = %v, want %v", got, want)
	}
	if len(m.SnapshotThreads()) != 0 {
		t.Fatal("closed issue still tracked")
	}
}

func TestMapSkipsStartupSummary(t *testing.T) {
	m := NewMap()
	tracker := &fakeTracker{}
	ctx := context.Background()
	summary := providertest.Summary()
	resolved := summary
	resolved.Revision, resolved.Status = 2, notification.StatusResolved
	for _, msg := range []notification.Message{summary, resolved} {
		if err := m.Deliver(ctx, tracker, msg, "t", "b"); err != nil {
			t.Fatal(err)
		}
	}
	if len(tracker.calls) != 0 || len(m.SnapshotThreads()) != 0 {
		t.Fatalf("summary reached the tracker: %v", tracker.calls)
	}
}

func TestMapIgnoresResolveWithoutIssue(t *testing.T) {
	tracker := &fakeTracker{}
	err := NewMap().Deliver(context.Background(), tracker,
		providertest.Resolve(), "t", "b")
	if err != nil || len(tracker.calls) != 0 {
		t.Fatalf("calls = %v, err = %v", tracker.calls, err)
	}
}

func TestMapRestoresIssuesAcrossRestart(t *testing.T) {
	m := NewMap()
	m.RestoreThreads(map[string]string{
		providertest.Announce().ThreadKey(): "7",
	})
	tracker := &fakeTracker{}
	err := m.Deliver(context.Background(), tracker,
		providertest.Update(), "t", "b")
	if err != nil {
		t.Fatal(err)
	}
	if len(tracker.calls) != 1 || tracker.calls[0] != "comment:7" {
		t.Fatalf("calls = %v", tracker.calls)
	}
}

func TestMapBoundsTrackedIssues(t *testing.T) {
	m := NewMap()
	saved := make(map[string]string)
	for i := 0; i < maxTrackedIssues+5; i++ {
		saved["k"+strings.Repeat("x", i)] = "1"
	}
	m.RestoreThreads(saved)
	if got := len(m.SnapshotThreads()); got != maxTrackedIssues {
		t.Fatalf("tracked %d issues, want %d", got, maxTrackedIssues)
	}
}

func TestTitleAndBodyRenderTheNarrative(t *testing.T) {
	msg := providertest.Announce()
	title := Title(msg, 20)
	if len(title) > 20 || !strings.HasPrefix(title, "🔴") {
		t.Fatalf("title = %q", title)
	}
	providertest.AssertOneLeadingEmoji(t, Title(msg, 255))
	body := Body(msg)
	providertest.AssertOneLeadingEmoji(t, body)
	for _, want := range []string{
		msg.Note, "```\npanic: out of memory\n```",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body %q lacks %q", body, want)
		}
	}
	if strings.Contains(body, "Cluster:") {
		t.Fatalf("body adds a cluster label line: %q", body)
	}
	if got := Body(providertest.Resolve()); got !=
		providertest.Resolve().Note {
		t.Fatalf("resolve body = %q", got)
	}
}

func TestFencedBodyUsesTheTrackerFence(t *testing.T) {
	body := FencedBody(providertest.Announce(), "{noformat}")
	if !strings.Contains(body, "{noformat}\npanic: out of memory\n{noformat}") {
		t.Fatalf("body = %q", body)
	}
}
