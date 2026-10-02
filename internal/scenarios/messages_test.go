package scenarios

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// messagesPath is where a scenario's delivered notes are kept.
func messagesPath(name string) string {
	return filepath.Join("testdata", "messages", name+".golden")
}

// scenarioNotes renders every message a replay delivered, one note per
// paragraph, with the time and the decision that sent it.
func scenarioNotes(t *testing.T, name string) string {
	t.Helper()
	log, e := loadScenario(t, name)
	result := replayLog(t, log, e.options(log.Start))
	var b strings.Builder
	for i, m := range result.Messages {
		b.WriteString(result.Times[i].Format("15:04:05") + " " +
			result.Decisions[i].Reason + "\n")
		b.WriteString(m.Note + "\n" + m.Short + "\n\n")
	}
	return b.String()
}

// TestScenarioMessagesGolden keeps the wording of every message each
// scenario sends. After a deliberate wording change, regenerate with:
//
//	go test ./internal/scenarios -run TestScenarioMessagesGolden -update
//
// and review the diff like any other code change.
func TestScenarioMessagesGolden(t *testing.T) {
	for _, s := range library() {
		name := s.expect.Name
		t.Run(name, func(t *testing.T) {
			got := scenarioNotes(t, name)
			path := messagesPath(name)
			if *update {
				writeGolden(t, path, got)
				return
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("missing golden file; run with -update: %v", err)
			}
			if got != string(want) {
				t.Errorf("messages changed\n got:\n%s\nwant:\n%s", got, want)
			}
		})
	}
}

func writeGolden(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

// unreadable matches what a person must never read in a note: explain's
// summaries ("explains 2 failures"), mode identifiers
// ("(Config.MemoryLimitTooLow)") and broken acronyms ("tLS").
var unreadable = regexp.MustCompile(
	`explains \d+ failure|\([A-Z][a-z]+\.[A-Z]\w*\)|\b[a-z][A-Z]{2,}\b`)

// TestScenarioMessagesAreReadable checks every scenario message for
// leaked identifiers and wrong resolutions.
func TestScenarioMessagesAreReadable(t *testing.T) {
	for _, s := range library() {
		name := s.expect.Name
		t.Run(name, func(t *testing.T) {
			log, e := loadScenario(t, name)
			result := replayLog(t, log, e.options(log.Start))
			for _, m := range result.Messages {
				if found := unreadable.FindString(m.Note); found != "" {
					t.Errorf("note shows %q: %s", found, m.Note)
				}
				if strings.Contains(m.Note, "Node") &&
					strings.Contains(m.Note, "gone") &&
					strings.Contains(m.Note, "healthy again") {
					t.Errorf("a deleted node is not healthy: %s", m.Note)
				}
			}
		})
	}
}
