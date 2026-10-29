package providertest

import (
	"strconv"
	"strings"

	"github.com/abahmed/kwatch/internal/notification"
)

// CutLevels names the evidence cut levels a Pro message goes through
// when it does not fit a provider: 1 trims the log quote from the top,
// 2 drops the last proof facts (spread and change), 3 cuts more proof
// from the bottom, 4 drops the quote, 5 shortens the cause. The fix is
// never cut.
var CutLevels = []int{1, 2, 3, 4, 5}

// CutLevel is a Pro message sized for limit so that fitting it reaches
// level. The same words at every level keep goldens readable.
func CutLevel(level, limit int) notification.Message {
	m := Announce()
	head := "🔴 api in shop was killed for using more memory than its " +
		"256Mi limit. Confirmed."
	proof, quote := []string{
		"Last exit: OOMKilled, exit code 137, 4 restarts in 40 minutes.",
		"The limit is 256Mi and the request is 128Mi.",
	}, []string{"cache warm: 41,882 entries", "cache warm: 96,310 entries"}
	switch level {
	case 1:
		quote = numbered("cache warm: entries ", 40, limit/30)
	case 2:
		proof = append(proof, padded("Also affected: web, worker", limit/2),
			padded("Changed: api:2.9 rolled out at 12:58 UTC", limit/2))
	case 3:
		proof = numbered("Memory climbed before kill ", 40, limit/20)
	case 4:
		quote = numbered("cache dump ", 2, limit)
	case 5:
		head = padded(head, limit*2)
		proof, quote = nil, nil
	}
	m.Short = head
	m.Doc = cutDoc(head, proof)
	m.Steps = []notification.Step{{Text: "Raise the limit",
		Command: FixCommand, Mutating: true}}
	m.Output = quote
	return m
}

func cutDoc(head string, proof []string) []notification.Block {
	doc := []notification.Block{{Kind: notification.Para,
		Spans: []notification.Span{{Text: head}}}}
	for _, p := range proof {
		doc = append(doc, notification.Block{Kind: notification.Bullet,
			Spans: []notification.Span{{Text: p}}})
	}
	return append(doc,
		notification.Block{Kind: notification.Para, Pinned: true,
			Spans: []notification.Span{{Text: "Raise the memory limit " +
				"to 384Mi, half again above the observed peak of " +
				"255Mi (changes the cluster):"}}},
		notification.Block{Kind: notification.CodeBlock, Pinned: true,
			Spans: []notification.Span{{Text: FixCommand}}})
}

// numbered is n lines of prefix plus a counter, each padded to width.
func numbered(prefix string, n, width int) []string {
	out := make([]string, 0, n)
	for i := 1; i <= n; i++ {
		out = append(out, padded(prefix+strconv.Itoa(i), width))
	}
	return out
}

// padded fills text with dots to width bytes.
func padded(text string, width int) string {
	if len(text) >= width {
		return text
	}
	return text + " " + strings.Repeat(".", width-len(text)-1)
}
