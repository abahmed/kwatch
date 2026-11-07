package safetext

import (
	"strings"

	"github.com/abahmed/kwatch/internal/notification"
)

// minOutputBudget is the least room worth spending on an output block;
// with less, the block is dropped and the Note keeps the space.
const minOutputBudget = 40

// DetailsLimit bounds the free-text details or description sent to a
// paging provider. GoAlert documents about 6 KiB for details; the other
// pagers accept at least that, so one conservative bound serves them all.
const DetailsLimit = 6000

// PlainWithOutput is note, then sep, then the output lines as plain text,
// cut to limit bytes. Pagers show it as text, so there is no fence to
// close; the cut keeps the note first so it is what survives.
func PlainWithOutput(
	note string, output []string, sep string, limit int,
) string {
	text := note
	if len(output) > 0 {
		text += sep + strings.Join(output, "\n")
	}
	return notification.Truncate(text, limit)
}

// LastOutput labels the output lines "Last output:" for plain-text pagers,
// or returns nil when there is no output.
func LastOutput(output []string) []string {
	if len(output) == 0 {
		return nil
	}
	return append([]string{"Last output:"}, output...)
}
