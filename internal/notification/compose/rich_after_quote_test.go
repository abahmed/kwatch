package compose

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/notification"
)

// longError is a pod's error too long for an inline span, so the rich
// message lifts it into a code block of its own.
const longError = `Startup probe failed: Get "http://10.244.0.12:8080/` +
	`healthz": context deadline exceeded (Client.Timeout exceeded)`

// strayLines are the lines of a rich message that start with the
// punctuation of a sentence a code block cut in two.
func strayLines(msg notification.Message) []string {
	var out []string
	for _, b := range msg.Doc {
		if b.Kind != notification.Para && b.Kind != notification.Small {
			continue
		}
		if text := b.Text(); strings.ContainsAny(text[:min(1, len(text))],
			".;,:") {
			out = append(out, text)
		}
	}
	return out
}

func docLines(msg notification.Message) []string {
	var out []string
	for _, b := range msg.Doc {
		out = append(out, b.Text())
	}
	return out
}

// The mention that follows a quotation lifted into a code block starts
// its own line; it does not begin with the stop that ended the quote.
func TestRichErrorMentionsStartsItsOwnLineAfterTheQuoteBlock(t *testing.T) {
	msg := Writer{}.Write(announce(secretMentioned(1, 1, longError)),
		at(6, 0))

	assert.Empty(t, strayLines(msg), docLines(msg))
	assert.Contains(t, docLines(msg), "The error mentions it.")
	assert.Contains(t, msg.Note, `". The error mentions it.`,
		"the plain note keeps one sentence")
}

// A clause that continues after a quotation in a code block starts its
// own sentence.
func TestRichClauseAfterTheQuoteBlockStartsASentence(t *testing.T) {
	msg := Writer{}.Write(announce(notReadyNode("False",
		"container runtime network not ready: NetworkReady=false "+
			"reason:NetworkPluginNotReady message:Network plugin returns "+
			"error: cni plugin not initialized and nothing else works",
		detection.Evidence{Label: detection.EvidenceNodeEvent,
			Value: `ImageGCFailed "failed to get imageFs info"`})),
		writerNow)

	assert.Empty(t, strayLines(msg), docLines(msg))
	assert.Contains(t, docLines(msg), "Its recent events include "+
		`ImageGCFailed "failed to get imageFs info".`)
}

// No message of the golden set has such a line.
func TestRichGoldenMessagesHaveNoStrayPunctuationLines(t *testing.T) {
	for _, c := range goldenCases() {
		t.Run(c.name, func(t *testing.T) {
			assert.Empty(t, strayLines(c.write()))
		})
	}
}
