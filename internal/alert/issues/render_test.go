package issues

import (
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/notification"
)

func msgWithOutput(lines ...string) notification.Message {
	return notification.Message{Note: "note", Output: lines}
}

func TestBodyFenceOutgrowsBackticksInOutput(t *testing.T) {
	cases := []struct {
		name  string
		lines []string
		fence string
	}{
		{"plain", []string{"hello"}, "```"},
		{"three", []string{"a ``` b"}, "````"},
		{"five", []string{"`````"}, "``````"},
		{"two stay short", []string{"``x``"}, "```"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := Body(msgWithOutput(tc.lines...))
			want := "note\n\n" + tc.fence + "\n" +
				strings.Join(tc.lines, "\n") + "\n" + tc.fence
			if body != want {
				t.Fatalf("body = %q, want %q", body, want)
			}
		})
	}
}

func TestFencedBodyNeutralisesNoformatToken(t *testing.T) {
	body := FencedBody(
		msgWithOutput("x {noformat} @everyone"), "{noformat}")
	if strings.Count(body, "{noformat}") != 2 {
		t.Fatalf("only the two real fences may remain: %q", body)
	}
	if !strings.Contains(body, "{\u200bnoformat}") {
		t.Fatalf("token not broken up: %q", body)
	}
}

func TestBodyWithoutOutputHasNoFence(t *testing.T) {
	if got := Body(notification.Message{Note: "n"}); got != "n" {
		t.Fatalf("body = %q", got)
	}
}

func TestNoteMentionsAreNeutralised(t *testing.T) {
	msg := notification.Message{
		Note:   "crashed: @alice and @org/team-a see user@example.com",
		Output: []string{"@bob in output"},
	}
	body := Body(msg)
	if strings.Contains(body[:strings.Index(body, "```")], "@a") ||
		strings.Contains(body, "@org") {
		t.Fatalf("a mention survived in the note: %q", body)
	}
	if !strings.Contains(body, "user@example.com") {
		t.Fatalf("an email address is not a mention: %q", body)
	}
	if !strings.Contains(body, "@\u200balice") {
		t.Fatalf("the mention must still read as text: %q", body)
	}
	if !strings.Contains(body, "@bob in output") {
		t.Fatalf("output is verbatim inside its fence: %q", body)
	}
}

func TestFencedBodyWithEscapesTheNote(t *testing.T) {
	msg := notification.Message{Note: "see [x]"}
	body := FencedBodyWith(msg, "{noformat}", strings.ToUpper)
	if body != "SEE [X]" {
		t.Fatalf("body = %q", body)
	}
}
