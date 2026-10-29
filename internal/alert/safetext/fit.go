package safetext

import (
	"strings"

	"github.com/abahmed/kwatch/internal/notification"
)

// minQuoteLines is the fewest quoted lines worth keeping: the signature
// line and the one before it.
const minQuoteLines = 2

// Pinned reports a Pro message whose fix is pinned.
func Pinned(doc []notification.Block) bool {
	return len(doc) > 0 && doc[len(doc)-1].Pinned
}

// FitPinned fits a Pro message into limit in the evidence cut order:
// first the quote loses lines from the top (keeping the last ones, which
// carry the signature) down to minQuoteLines; then the proof is cut from
// the bottom, the fix staying whole; then the quote is dropped; last the
// headline itself is cut. The cause and the fix are never cut while
// anything else is left.
func FitPinned(
	m notification.Message, d notification.Dialect, sep string, limit int,
) string {
	note := m.Render(d)
	lines := m.Output
	for n := len(lines); n >= minQuoteLines; n-- {
		quote := d.Fence(neutral(d, strings.Join(lines[len(lines)-n:], "\n")))
		if len(note)+len(sep)+len(quote) <= limit {
			return note + sep + quote
		}
	}
	quote := d.Fence(neutral(d, strings.Join(
		lines[max(0, len(lines)-minQuoteLines):], "\n")))
	if room := limit - len(sep) - len(quote); room > 0 {
		cut := m.RenderWithin(d, room)
		if strings.HasSuffix(cut, lastBlock(m, d)) {
			return cut + sep + quote
		}
	}
	return m.RenderWithin(d, limit)
}

// lastBlock is the pinned fix's command as rendered, to tell a cut that
// kept the fix from one that had to give it up.
func lastBlock(m notification.Message, d notification.Dialect) string {
	return notification.RenderBlocks(m.Doc[len(m.Doc)-1:], d)
}
