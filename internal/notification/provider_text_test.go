package notification

import (
	"strings"
	"testing"
)

func TestMessageNoteTextFallsBackToText(t *testing.T) {
	m := Message{Status: StatusCritical, Title: "api is down"}
	if got := m.NoteText(); got != "🔴 api is down" {
		t.Fatalf("NoteText() = %q", got)
	}
	m.Note = "🔴 api is down since 10:00."
	if got := m.NoteText(); got != m.Note {
		t.Fatalf("NoteText() = %q", got)
	}
}

func TestMessageShortTextPrefersShort(t *testing.T) {
	m := Message{Marker: MarkerLow, Title: "disk is filling"}
	if got := m.ShortText(); got != "🟡 disk is filling" {
		t.Fatalf("ShortText() = %q", got)
	}
	m.Short = "🟡 disk is 91% full."
	if got := m.ShortText(); got != m.Short {
		t.Fatalf("ShortText() = %q", got)
	}
}

func TestMessageAlertKeyIsStable(t *testing.T) {
	if got := (Message{Key: "p-1"}).AlertKey(""); got != "kwatch-p-1" {
		t.Fatalf("AlertKey() = %q", got)
	}
	if got := (Message{}).AlertKey(""); got != "kwatch-notice" {
		t.Fatalf("empty AlertKey() = %q", got)
	}
	if !(Message{Status: StatusResolved}).Resolved() {
		t.Fatal("resolved status must report Resolved")
	}
}

func TestMessageAlertKeyIncludesCluster(t *testing.T) {
	m := Message{Key: "p-1"}
	prod, staging := m.AlertKey("prod"), m.AlertKey("staging")
	if prod != "kwatch-prod-p-1" || prod == staging {
		t.Fatalf("AlertKey(prod) = %q, AlertKey(staging) = %q",
			prod, staging)
	}
	if got := m.AlertKey(" eu west "); got != "kwatch-eu-west-p-1" {
		t.Fatalf("AlertKey with spaces = %q", got)
	}
	if got := m.ThreadKey(); got != "kwatch-p-1" {
		t.Fatalf("ThreadKey() = %q", got)
	}
	if m.IsNotice() || !(Message{}).IsNotice() {
		t.Fatal("IsNotice must only match notice keys")
	}
}

func TestNoticeUsesFirstLineAsShort(t *testing.T) {
	n := Notice("kwatch started\nversion 1")
	if n.Short != "kwatch started" || n.Note != "kwatch started\nversion 1" {
		t.Fatalf("Notice() = %+v", n)
	}
	if n.AlertKey("") != "kwatch-notice" || n.Resolved() || !n.IsNotice() {
		t.Fatalf("Notice identity = %q", n.AlertKey(""))
	}
}

func TestNoticeKeepsItsLeadingMarker(t *testing.T) {
	cases := map[string]struct {
		text   string
		status Status
		title  string
	}{
		"low": {"🟡 kwatch v1.2.0 started.", StatusLow,
			"kwatch v1.2.0 started."},
		"notify": {"🟠 kwatch started late.", StatusWarning,
			"kwatch started late."},
		"no marker": {"kwatch started.", StatusWarning,
			"kwatch started."},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			n := Notice(c.text)
			if n.Status != c.status || n.Title != c.title {
				t.Fatalf("Notice(%q) = %+v", c.text, n)
			}
			if n.ShortText() != c.text || n.NoteText() != c.text {
				t.Fatalf("notice text changed: %q / %q",
					n.ShortText(), n.NoteText())
			}
		})
	}
}

func TestTruncateCutsOnRuneBoundary(t *testing.T) {
	cases := []struct {
		text  string
		limit int
		want  string
	}{
		{"short", 10, "short"},
		{"hello world", 8, "hello…"},
		{"héllo", 5, "h…"},
		{"hello", 2, ""},
		{"hello", 0, "hello"},
	}
	for _, tc := range cases {
		if got := Truncate(tc.text, tc.limit); got != tc.want {
			t.Errorf("Truncate(%q, %d) = %q, want %q",
				tc.text, tc.limit, got, tc.want)
		}
		if tc.limit > 0 && len(Truncate(tc.text, tc.limit)) > tc.limit {
			t.Errorf("Truncate(%q, %d) exceeds limit", tc.text, tc.limit)
		}
	}
}

func TestMailSubjectAndBody(t *testing.T) {
	m := Message{
		Short: "🔴 api\r\nBcc: x@example.com", Note: "🔴 api is down.",
		Output: []string{"panic: boom", "exit 2"},
	}
	if got := m.MailSubject(); got != "🔴 api Bcc: x@example.com" {
		t.Fatalf("MailSubject() = %q", got)
	}
	want := "🔴 api is down.\n\n> panic: boom\n> exit 2"
	if got := m.MailBody(); got != want {
		t.Fatalf("MailBody() = %q", got)
	}
	m.Output = nil
	if got := m.MailBody(); got != m.Note {
		t.Fatalf("MailBody() without output = %q", got)
	}
}

func TestTruncateKeepsMarkupWellFormed(t *testing.T) {
	cases := []struct {
		name, text string
		limit      int
	}{
		{"open fence", "intro\n```\nline one\nline two\nline three\n```\nend", 30},
		{"cut inside a fence marker", "ab```code```cd", 7},
		{"cut inside a tag", "<b>bold</b> and <a href=\"http://x\">link</a>", 28},
		{"closed fence kept", "```\nok\n```\nand a long tail of text", 20},
	}
	for _, tc := range cases {
		got := Truncate(tc.text, tc.limit)
		if len(got) > tc.limit {
			t.Errorf("%s: %q is %d bytes, limit %d",
				tc.name, got, len(got), tc.limit)
		}
		if strings.Count(got, "```")%2 != 0 {
			t.Errorf("%s: unclosed fence in %q", tc.name, got)
		}
		if i := strings.LastIndex(got, "<"); i >= 0 &&
			!strings.Contains(got[i:], ">") {
			t.Errorf("%s: %q ends inside a tag", tc.name, got)
		}
	}
}

// A cut that leaves a code block open closes it, within the limit, and
// a cut after the block leaves nothing to close.
func TestTruncateClosesAnOpenFence(t *testing.T) {
	text := "```\n" + strings.Repeat("line of output\n", 20)
	got := Truncate(text, 60)

	if !strings.HasSuffix(got, "…\n```") {
		t.Fatalf("fence not closed after the ellipsis: %q", got)
	}
	if len(got) > 60 {
		t.Fatalf("%d bytes, limit 60", len(got))
	}
	closed := "```\nok\n```\n" + strings.Repeat("tail ", 20)
	if got := Truncate(closed, 40); strings.HasSuffix(got, "\n```") {
		t.Fatalf("a closed block got a second fence: %q", got)
	}
}
