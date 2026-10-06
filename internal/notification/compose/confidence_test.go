package compose

import (
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/rootcause"
)

func secretCause(score float64) incident.Incident {
	p := podCrash(incident.Notify)
	p.Root = inventory.CoreID("secret", "default", "db")
	p.Cause = &rootcause.CauseRecord{Root: p.Root, Score: score,
		Summary: "secret db is missing"}
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

func TestCloseRivalsAreTwoPossibleCauses(t *testing.T) {
	p := secretCause(0.74)
	p.Cause.Rival = &rootcause.CauseRecord{
		Root:  inventory.CoreID("configmap", "default", "cfg"),
		Score: 0.72, Summary: "configmap cfg is missing"}

	msg := Writer{}.Write(announce(p), writerNow)

	want := "Pod api-1 in default is crash looping; two possible " +
		"causes: secret db is missing or configmap cfg is missing."
	if msg.Title != want {
		t.Errorf("Title = %q, want %q", msg.Title, want)
	}
	if msg.Confidence != "possible" {
		t.Errorf("Confidence = %q, a pair is never stated as high",
			msg.Confidence)
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

	want := "Checked: image api:2.3 runs fine in staging-2; node n3 is " +
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
	if strings.Contains(msg.Note, "Checked:") {
		t.Errorf("no checks, no line: %s", msg.Note)
	}
}
