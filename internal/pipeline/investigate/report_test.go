package investigate

import (
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/incident"
)

// A proof cut inside the log line it quotes still closes the quote.
func TestBoundCorrelationClosesAQuoteTheClipCut(t *testing.T) {
	proof := "the last run of app exited with code 2 after logging \"" +
		strings.Repeat("panic: cannot connect ", 20) + "\""
	got := boundCorrelation(&incident.Correlation{Confirmed: proof})

	if !strings.HasSuffix(got.Confirmed, "…\"") {
		t.Fatalf("quote left open: %q", got.Confirmed)
	}
	whole := "it logged \"boom\""
	if got := clipQuoted(whole); got != whole {
		t.Fatalf("closed quote changed: %q", got)
	}
}

// Escaped quotes inside the quoted line do not count, and closing the
// quote never makes the proof longer than the clip.
func TestClipQuotedIgnoresEscapedQuotesAndKeepsTheBound(t *testing.T) {
	limit := len(clip(strings.Repeat("x", maxEvidenceText+50)))
	open := "logged \"" + strings.Repeat(`a\"b `, 60) + "\""
	got := clipQuoted(open)
	if !strings.HasSuffix(got, "…\"") || len(got) > limit {
		t.Fatalf("open quote: %d bytes, %q", len(got), got)
	}
	closed := "logged \"x\" then " + strings.Repeat(`a\"b `, 60)
	got = clipQuoted(closed)
	if strings.HasSuffix(got, "\"") || len(got) > limit {
		t.Fatalf("closed quote: %d bytes, %q", len(got), got)
	}
}
