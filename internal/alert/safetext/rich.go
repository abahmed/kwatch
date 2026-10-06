package safetext

import (
	"strings"

	"github.com/abahmed/kwatch/internal/notification"
)

// RichWithOutput writes the message in the dialect, then sep, then the
// application output as a code block of the same dialect. It is
// NoteWithOutput for the structured message: every piece of text is
// escaped by the dialect, the output is cut before its fence is added,
// and the note is cut by whole blocks, so no cut lands inside markup.
// limit bounds the result in bytes (0 means no bound).
func RichWithOutput(
	m notification.Message, d notification.Dialect, sep string, limit int,
) string {
	if len(m.Output) == 0 {
		return m.RenderWithin(d, limit)
	}
	raw := strings.Join(m.Output, "\n")
	if limit <= 0 {
		return m.Render(d) + sep + d.Fence(neutral(d, raw))
	}
	// The note comes first: when it leaves too little room, the output
	// is dropped and the note keeps the space.
	note := m.RenderWithin(d, limit)
	for len(raw) >= minOutputBudget {
		block := d.Fence(neutral(d, raw))
		if len(note)+len(sep)+len(block) <= limit {
			return note + sep + block
		}
		raw = notification.Truncate(raw, len(raw)*3/4)
	}
	return note
}

func neutral(d notification.Dialect, s string) string {
	if d.Neutralize == nil {
		return s
	}
	return d.Neutralize(s)
}
