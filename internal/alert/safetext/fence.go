package safetext

import (
	"strings"

	"github.com/abahmed/kwatch/internal/notification"
)

// minFence is the shortest Markdown code fence.
const minFence = 3

// minOutputBudget is the least room worth spending on an output block;
// with less, the block is dropped and the Note keeps the space.
const minOutputBudget = 40

// NoteWithOutput is note, then sep, then the application output as a code
// block. Output is workload text, so the block's fence is made longer than
// any backtick run inside it: the output cannot close the block and turn
// the rest into live markup.
//
// limit bounds the result in bytes (0 means no bound). The output is cut
// before the closing fence is added, so the fence is never the part that
// is cut off. Callers neutralize mentions in note and output first, since
// that can lengthen the text.
func NoteWithOutput(
	note string, output []string, sep string, limit int,
) string {
	if len(output) == 0 {
		return notification.Truncate(note, limit)
	}
	body := strings.Join(output, "\n")
	if limit <= 0 {
		return note + sep + fenced(body)
	}
	// The fence is at most one backtick longer than the longest run in
	// the uncut body, which bounds the room the fences and newlines take.
	overhead := len(sep) + 2*fenceLen(body) + 2
	budget := limit - len(note) - overhead
	if budget < minOutputBudget {
		return notification.Truncate(note, limit)
	}
	return note + sep + fenced(notification.Truncate(body, budget))
}

// fenced wraps body in a code fence longer than any backtick run in it.
func fenced(body string) string {
	fence := strings.Repeat("`", fenceLen(body))
	return fence + "\n" + body + "\n" + fence
}

// fenceLen is the shortest safe fence length for body.
func fenceLen(body string) int {
	if longest := longestRun(body, '`'); longest >= minFence {
		return longest + 1
	}
	return minFence
}

// longestRun is the length of the longest run of c in text.
func longestRun(text string, c byte) int {
	longest, run := 0, 0
	for i := 0; i < len(text); i++ {
		if text[i] != c {
			run = 0
			continue
		}
		run++
		if run > longest {
			longest = run
		}
	}
	return longest
}

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
