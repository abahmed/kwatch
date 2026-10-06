package replay_test

import (
	"context"
	"flag"
	"os"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/replay"
)

const badRolloutLog = "testdata/bad_rollout.jsonl"

var update = flag.Bool("update", false, "rewrite testdata logs")

// TestReplayBadRolloutFixture regenerates the committed log with -update:
//
//	go test ./internal/replay -run BadRollout -update
func TestReplayBadRolloutFixture(t *testing.T) {
	if *update {
		if err := os.WriteFile(badRolloutLog, recordBadRollout(t),
			0o600); err != nil {
			t.Fatal(err)
		}
	}
	result := replayFile(t, badRolloutLog)
	if len(result.Decisions) == 0 {
		t.Fatal("expected the rollout incident to be announced")
	}
	first := result.Decisions[0]
	if first.Action != incident.Announce || first.Incident.Cause == nil ||
		first.Incident.Cause.Change == nil {
		t.Fatalf("first decision should announce a change cause, got %+v",
			first)
	}
	wantRoot := "deployment/shop/payments"
	for _, d := range result.Decisions {
		if got := d.Incident.Root.String(); got != wantRoot {
			t.Fatalf("decision rooted at %s, want %s", got, wantRoot)
		}
		if d.Incident.Root.Kind != kube.KindDeployment {
			t.Fatalf("root kind %s", d.Incident.Root.Kind)
		}
	}
	title := result.Messages[0].Title
	if !strings.Contains(title, "2.3") || !strings.Contains(title, "payments") {
		t.Fatalf("title %q should name the workload and the new image", title)
	}
	if got := actions(result); got != "announce" {
		t.Fatalf("decisions = %q, want one announcement", got)
	}
}

func TestReplayIsDeterministic(t *testing.T) {
	first := summarise(replayFile(t, badRolloutLog))
	for range 3 {
		if again := summarise(replayFile(t, badRolloutLog)); again != first {
			t.Fatalf("replays differ:\n%s\n---\n%s", first, again)
		}
	}
}

// A sanitized log still tells the same story: the same decisions, rooted
// at the pseudonym of the same deployment.
func TestReplaySanitizedLogKeepsTheStory(t *testing.T) {
	raw := replayFile(t, badRolloutLog)
	log := readFile(t, badRolloutLog)
	sanitizer, err := replay.NewSanitizer(
		replay.SanitizeOptions{Salt: "test"})
	if err != nil {
		t.Fatal(err)
	}
	var clean replay.Result
	clean, err = replay.Run(context.Background(), sanitizer.Log(log),
		newDependencies(), replay.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if actions(clean) != actions(raw) {
		t.Fatalf("sanitized decisions %q, raw %q", actions(clean),
			actions(raw))
	}
	root := clean.Decisions[0].Incident.Root
	if root.Kind != kube.KindDeployment || root.Name == "payments" ||
		root.Namespace == "shop" {
		t.Fatalf("sanitized root %s", root)
	}
	if strings.Contains(clean.Messages[0].Title, "payments") {
		t.Fatalf("sanitized title leaks a name: %q", clean.Messages[0].Title)
	}
}

func readFile(t *testing.T, path string) replay.Log {
	t.Helper()
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	log, err := replay.Read(file)
	if err != nil {
		t.Fatal(err)
	}
	return log
}

func replayFile(t *testing.T, path string) replay.Result {
	t.Helper()
	result, err := replay.Run(context.Background(), readFile(t, path),
		newDependencies(), replay.Options{})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func actions(result replay.Result) string {
	names := map[incident.Action]string{
		incident.Announce: "announce", incident.Update: "update",
		incident.Resolve: "resolve",
	}
	parts := make([]string, 0, len(result.Decisions))
	for _, d := range result.Decisions {
		parts = append(parts, names[d.Action])
	}
	return strings.Join(parts, ",")
}

// summarise renders what a person would see, so two replays compare as
// text.
func summarise(result replay.Result) string {
	var b strings.Builder
	for i, d := range result.Decisions {
		m := result.Messages[i]
		b.WriteString(strings.Join([]string{
			d.Incident.ID, d.Incident.Root.String(), string(d.Reason),
			d.Incident.Opened.String(), d.Incident.Announced.String(),
			m.Key, m.Title, strings.Join(m.Lines, "|"),
			strings.Join(m.Timeline, "|"),
		}, " ; "))
		b.WriteString("\n")
	}
	b.WriteString(result.End.String())
	return b.String()
}
