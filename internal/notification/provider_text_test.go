package notification

import "testing"

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
