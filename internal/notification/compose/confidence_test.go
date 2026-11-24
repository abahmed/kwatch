package compose

import (
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// secretCause is a pod crash blamed on secret db at score, with one
// proof for it, so a possible cause is corroborated.
func secretCause(score float64) incident.Incident {
	p := podCrash(incident.Notify)
	p.Root = inventory.CoreID("secret", "default", "db")
	p.Cause = &rootcause.CauseRecord{Root: p.Root, Score: score,
		Summary: "secret db is missing",
		Proof:   []rootcause.Proof{{Text: "it is missing", Supports: true}}}
	return p
}

func TestLeadWordsFollowTheScorecardLevels(t *testing.T) {
	tests := []struct {
		name  string
		score float64
		want  string
	}{
		{"high asserts", 0.72, "crash looping because secret db"},
		{"likely says likely", 0.6, "crash looping, likely because secret"},
		{"just under high is likely", 0.69,
			"crash looping, likely because secret"},
		{"possible only suggests", 0.46,
			"crash looping; secret db is missing, which might be related"},
	}
	// Without the proof nothing corroborates a possible cause: it is
	// not named as the cause.
	uncorroborated := secretCause(0.46)
	uncorroborated.Cause.Proof = nil
	msg := Writer{}.Write(announce(uncorroborated), writerNow)
	if !strings.HasSuffix(msg.Title, "; kwatch could not confirm a "+
		"cause.") || strings.Contains(msg.Title, "secret db") {
		t.Errorf("Title = %q names an uncorroborated cause", msg.Title)
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg := Writer{}.Write(announce(secretCause(tt.score)), writerNow)
			if !strings.Contains(msg.Title, tt.want) {
				t.Errorf("Title = %q, want it to contain %q", msg.Title,
					tt.want)
			}
		})
	}
}

// Two causes that scored almost alike are neither named as the cause:
// the lead says the symptom and that no cause was confirmed, and only
// the closing line names both, as possibilities.
func TestCloseRivalsAreNotNamedAsTheCause(t *testing.T) {
	p := secretCause(0.74)
	p.Cause.Rival = &rootcause.CauseRecord{
		Root:  inventory.CoreID("configmap", "default", "cfg"),
		Score: 0.72, Summary: "configmap cfg is missing"}

	msg := Writer{}.Write(announce(p), writerNow)

	want := "Pod api-1 in default is crash looping; kwatch could not " +
		"confirm a cause."
	if msg.Title != want {
		t.Errorf("Title = %q, want %q", msg.Title, want)
	}
	if !strings.HasSuffix(msg.Note, "Possibilities kwatch could not "+
		"confirm: secret db is missing or configmap cfg is missing.") {
		t.Errorf("the possibilities close the message: %s", msg.Note)
	}
	if msg.Confidence != "" {
		t.Errorf("Confidence = %q, no cause is stated", msg.Confidence)
	}
	if strings.Contains(msg.Note, "Rolling back") {
		t.Errorf("no fix is offered for an undecided cause: %s", msg.Note)
	}
}

func TestCheckedLineListsAtMostThreeChecks(t *testing.T) {
	p := secretCause(0.8)
	p.Cause.Checked = []string{
		"image api:2.3 runs fine in staging-2.",
		"node n3 is healthy for 12 other pods",
		"the previous revision ran healthy for 9 minutes",
		"a fourth check that is never shown",
	}

	msg := Writer{}.Write(announce(p), writerNow)

	want := "Proof: image api:2.3 runs fine in staging-2; node n3 is " +
		"healthy for 12 other pods; the previous revision ran healthy " +
		"for 9 minutes."
	if !strings.HasSuffix(msg.Note, want) {
		t.Fatalf("Note should end with %q:\n%s", want, msg.Note)
	}
	if strings.Contains(msg.Note, "fourth") {
		t.Errorf("only three checks are shown: %s", msg.Note)
	}
	last := msg.Doc[len(msg.Doc)-1]
	if last.Kind != notification.Small || last.Text() != want {
		t.Fatalf("last block = %+v, want small print", last)
	}
	slack := msg.Render(notification.SlackDialect())
	if !strings.HasSuffix(slack, "_"+want+"_") {
		t.Errorf("Slack should italicise the line:\n%s", slack)
	}
	plain := msg.Render(notification.PlainDialect())
	if !strings.HasSuffix(plain, want) {
		t.Errorf("plain should end with the line:\n%s", plain)
	}
}

func TestNoChecksNoCheckedLine(t *testing.T) {
	msg := Writer{}.Write(announce(secretCause(0.8)), writerNow)
	if strings.Contains(msg.Note, "Proof:") {
		t.Errorf("no checks, no line: %s", msg.Note)
	}
}
