package compose

import (
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/notification"
)

// This file turns the finished sentences into notification.Doc: the
// same words, split into short lines, with the meaning of each span
// (a resource name, text a pod wrote, a command) marked. Renderers
// choose the provider's syntax; nothing here writes any.

// docOf is the structured form of a message: the headline, then one
// line per sentence. The action sentence ends in a command; the command
// moves to its own code block. A closing sentence in the action part
// with no command is small print.
func docOf(
	msg notification.Message, marker string, sentences []sentence,
) []notification.Block {
	if len(sentences) == 0 {
		return nil
	}
	headline := notification.Block{Kind: notification.Para,
		Spans: append([]notification.Span{{Text: marker + " "}},
			podSpans(sentences[0].text)...)}
	doc := []notification.Block{headline}
	for _, s := range sentences[1:] {
		doc = append(doc, sentenceBlocks(s, msg.Steps)...)
	}
	return doc
}

// sentenceBlocks is one sentence as a line, or as a line and a code
// block when it ends with the suggested command.
func sentenceBlocks(
	s sentence, steps []notification.Step,
) []notification.Block {
	if s.part == partChecked {
		return []notification.Block{{Kind: notification.Small,
			Spans: podSpans(s.text)}}
	}
	if s.part != partAction {
		return paragraphs(s.text)
	}
	for _, step := range steps {
		cmd := strings.TrimSpace(step.Command)
		if cmd == "" || !strings.HasSuffix(s.text, cmd) {
			continue
		}
		lead := strings.TrimSpace(strings.TrimSuffix(s.text, cmd))
		return []notification.Block{
			{Kind: notification.Para, Spans: podSpans(lead)},
			{Kind: notification.CodeBlock,
				Spans: []notification.Span{{Text: cmd}}},
		}
	}
	if s.text == eachOwnMessage {
		return []notification.Block{{Kind: notification.Small,
			Spans: podSpans(s.text)}}
	}
	return []notification.Block{{Kind: notification.Para,
		Spans: podSpans(s.text)}}
}

// longQuote is the length from which text a pod wrote gets a code block
// of its own instead of an inline span.
const longQuote = 80

// paragraphs is a sentence as a line. Quoted pod text that is long or
// has several lines is lifted out into a code block after the words
// that introduce it.
func paragraphs(text string) []notification.Block {
	parts := strings.Split(text, `"`)
	if len(parts)%2 == 0 {
		return []notification.Block{{Kind: notification.Para,
			Spans: podSpans(text)}}
	}
	var out []notification.Block
	line := ""
	flush := func() {
		if strings.TrimSpace(strings.Trim(line, " .,;:")) != "" {
			out = append(out, notification.Block{Kind: notification.Para,
				Spans: podSpans(strings.TrimSpace(line))})
		}
		line = ""
	}
	for i, part := range parts {
		if i%2 == 0 {
			line += part
			continue
		}
		if len([]rune(part)) <= longQuote && !strings.Contains(part, "\n") {
			line += `"` + part + `"`
			continue
		}
		flush()
		out = append(out, notification.Block{Kind: notification.CodeBlock,
			Spans: []notification.Span{{Text: part}}})
	}
	flush()
	return out
}

// podSpans splits text at its double quotes: what a pod wrote sits
// between them (see outsideQuotes) and becomes a code span, without the
// quotation marks. Unbalanced quotes leave the text plain.
func podSpans(text string) []notification.Span {
	parts := strings.Split(text, `"`)
	if len(parts)%2 == 0 {
		return []notification.Span{{Text: text}}
	}
	var spans []notification.Span
	for i, part := range parts {
		if part == "" {
			continue
		}
		style := notification.Plain
		if i%2 == 1 {
			style = notification.Code
		}
		spans = append(spans, notification.Span{Text: part, Style: style})
	}
	return spans
}

// nameStoplist holds kind words that are never bolded as names.
var nameStoplist = map[string]bool{
	"service": true, "ingress": true, "node": true, "pod": true,
	"deployment": true, "namespace": true, "job": true, "the": true,
}

// boldNames marks every whole-word occurrence of a name in the plain
// spans of the blocks as bold. Code spans and code blocks are never
// touched, and neither is small print, which is already set apart.
func boldNames(blocks []notification.Block, names []string) {
	names = usableNames(names)
	if len(names) == 0 {
		return
	}
	for i := range blocks {
		if blocks[i].Kind == notification.CodeBlock ||
			blocks[i].Kind == notification.Small {
			continue
		}
		var spans []notification.Span
		for _, s := range blocks[i].Spans {
			if s.Style != notification.Plain {
				spans = append(spans, s)
				continue
			}
			spans = append(spans, splitNames(s.Text, names)...)
		}
		blocks[i].Spans = spans
	}
}

// usableNames keeps names worth bolding, longest first so a longer
// name wins over a shorter one it contains.
func usableNames(names []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, n := range names {
		n = strings.TrimSpace(n)
		if utf8.RuneCountInString(n) < 3 || seen[n] ||
			nameStoplist[strings.ToLower(n)] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return len(out[i]) > len(out[j])
	})
	return out
}

// isNameRune is a character that can be part of a resource name or of
// the image, path or address it sits in.
func isNameRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) ||
		strings.ContainsRune("_-:/", r)
}

// splitNames cuts text into plain and bold spans around whole-word
// names. A name followed by "." is whole only at the end of a sentence.
func splitNames(text string, names []string) []notification.Span {
	var spans []notification.Span
	plainFrom := 0
	for i := 0; i < len(text); {
		n := nameAt(text, i, names)
		if n == "" {
			_, size := utf8.DecodeRuneInString(text[i:])
			i += size
			continue
		}
		if i > plainFrom {
			spans = append(spans, notification.Span{
				Text: text[plainFrom:i]})
		}
		spans = append(spans, notification.Span{Text: n,
			Style: notification.Bold})
		i += len(n)
		plainFrom = i
	}
	if plainFrom < len(text) {
		spans = append(spans, notification.Span{Text: text[plainFrom:]})
	}
	return spans
}

// nameAt is the name that starts at text[i] as a whole word, or "".
func nameAt(text string, i int, names []string) string {
	if i > 0 {
		before, _ := utf8.DecodeLastRuneInString(text[:i])
		if isNameRune(before) || before == '.' {
			return ""
		}
	}
	for _, n := range names {
		if !strings.HasPrefix(text[i:], n) {
			continue
		}
		rest := text[i+len(n):]
		after, size := utf8.DecodeRuneInString(rest)
		if rest != "" && isNameRune(after) {
			continue
		}
		if after == '.' {
			next, _ := utf8.DecodeRuneInString(rest[size:])
			if isNameRune(next) {
				continue
			}
		}
		return n
	}
	return ""
}

// incidentNames are the resource names of an incident worth bolding:
// its root and members, their namespaces and the cluster.
func incidentNames(p incident.Incident, cluster string) []string {
	names := []string{cluster, p.Root.Name, p.Root.Namespace}
	for _, m := range p.Members {
		names = append(names, m.Entity.Name, m.Entity.Namespace)
	}
	for _, id := range p.Impact {
		names = append(names, id.Name)
	}
	return names
}
