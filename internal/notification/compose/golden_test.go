package compose

import (
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/abahmed/kwatch/internal/notification"
)

// Run with -update to rewrite testdata/*.golden after a deliberate
// wording change, then review the diff like any other code change.
var update = flag.Bool("update", false, "rewrite golden notes")

func goldenText(m notification.Message) string {
	return m.Note + "\n\n" + m.Short + "\n"
}

func TestWriterGoldenNotes(t *testing.T) {
	for _, c := range goldenCases() {
		t.Run(c.name, func(t *testing.T) {
			got := goldenText(c.write())
			path := filepath.Join("testdata", c.name+".golden")
			if *update {
				if err := os.WriteFile(path, []byte(got), 0o600); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("missing golden file; run with -update: %v", err)
			}
			if got != string(want) {
				t.Fatalf("note changed\n got: %s\nwant: %s", got, want)
			}
		})
	}
}

// labelStart matches the section labels the note must not use.
var labelStart = regexp.MustCompile(`(?i)(^|\n|\. )(cause|impact|why|` +
	`evidence|confidence|next steps|timeline|how we know|affected|` +
	`started|root cause|resolved):`)

func TestWriterNoteProperties(t *testing.T) {
	for _, c := range goldenCases() {
		t.Run(c.name, func(t *testing.T) {
			m := c.write()
			assertOneEmoji(t, m.Note)
			assertOneEmoji(t, m.Short)
			if !strings.HasPrefix(m.Note, m.Marker+" ") {
				t.Errorf("note does not start with its marker: %q", m.Note)
			}
			if labelStart.MatchString(m.Note) {
				t.Errorf("note uses a label: %q", m.Note)
			}
			if strings.Contains(m.Note, "\n") ||
				strings.Contains(m.Note, "• ") {
				t.Errorf("note has lines or bullets: %q", m.Note)
			}
			for _, banned := range []string{"http://", "https://", "/why"} {
				if strings.Contains(m.Note, banned) {
					t.Errorf("note contains %q: %q", banned, m.Note)
				}
			}
			assertSentences(t, m.Note)
		})
	}
}

func assertOneEmoji(t *testing.T, text string) {
	t.Helper()
	count := 0
	for _, r := range text {
		if isEmoji(r) {
			count++
		}
	}
	if count != 1 {
		t.Errorf("want exactly one emoji, got %d: %q", count, text)
	}
	first, _ := utf8.DecodeRuneInString(text)
	if !isEmoji(first) {
		t.Errorf("emoji is not at the start: %q", text)
	}
}

func isEmoji(r rune) bool {
	return (r >= 0x1F000 && r <= 0x1FAFF) || (r >= 0x2600 && r <= 0x27BF) ||
		(r >= 0x2B00 && r <= 0x2BFF) || r == 0xFE0F
}

func assertSentences(t *testing.T, note string) {
	t.Helper()
	_, body, _ := strings.Cut(note, " ")
	seen := map[string]bool{}
	for _, s := range strings.Split(body, ". ") {
		s = strings.TrimSpace(s)
		if strings.Trim(s, ".;,: ") == "" {
			t.Errorf("empty sentence in %q", note)
		}
		if seen[s] {
			t.Errorf("repeated sentence %q in %q", s, note)
		}
		seen[s] = true
	}
	if strings.Contains(note, "..") || strings.Contains(note, " .") {
		t.Errorf("broken punctuation in %q", note)
	}
}
