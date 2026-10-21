package compose

import (
	"strings"

	"github.com/abahmed/kwatch/internal/notification"
)

// Text a pod wrote is wrapped in two control runes while a sentence is
// built. Cluster text can hold any number of double quotes, so a double
// quote cannot tell where a quotation ends; these two never occur in it
// (clip removes them). fill swaps them for double quotes in every plain
// text field, and the rich lines split on them.
const (
	quoteOpen  = "\x1e"
	quoteClose = "\x1f"
)

// markQuote wraps a payload in the quote runes.
func markQuote(payload string) string {
	return quoteOpen + payload + quoteClose
}

// plainQuotes writes the quote runes of text as double quotes, the form
// of every plain text field.
func plainQuotes(text string) string {
	if !strings.ContainsAny(text, quoteOpen+quoteClose) {
		return text
	}
	return strings.NewReplacer(quoteOpen, `"`, quoteClose, `"`).Replace(text)
}

// settleStop drops the full stop kwatch adds after a quotation that ends
// with one of its own: it reads `It fails with "boom."`, not `"boom.".`
// The quoted bytes are not touched; only kwatch's own stop goes.
func settleStop(text string) string {
	if !strings.Contains(text, "."+quoteClose+".") {
		return text
	}
	return strings.ReplaceAll(text, "."+quoteClose+".", "."+quoteClose)
}

// dropQuoteRunes removes the quote runes from text that came from a
// cluster, so it can never close or open a quotation of its own.
func dropQuoteRunes(text string) string {
	if !strings.ContainsAny(text, quoteOpen+quoteClose) {
		return text
	}
	return strings.NewReplacer(quoteOpen, "", quoteClose, "").Replace(text)
}

// quoteSegment is a part of a sentence: plain words, or the payload of a
// quotation.
type quoteSegment struct {
	text   string
	quoted bool
}

// segmentsOf splits text at its quote runes. A quotation that is never
// closed stays plain text. A stray rune inside a part (a second open, a
// close with no open) is dropped, so no part can carry one into a field.
func segmentsOf(text string) []quoteSegment {
	var out []quoteSegment
	for text != "" {
		open := strings.Index(text, quoteOpen)
		if open < 0 {
			break
		}
		end := strings.Index(text[open+1:], quoteClose)
		if end < 0 {
			break
		}
		if open > 0 {
			out = append(out,
				quoteSegment{text: dropQuoteRunes(text[:open])})
		}
		payload := dropQuoteRunes(text[open+1 : open+1+end])
		out = append(out, quoteSegment{text: payload, quoted: true})
		text = text[open+1+end+1:]
	}
	if text != "" {
		out = append(out, quoteSegment{text: dropQuoteRunes(text)})
	}
	return out
}

// outsideQuotes applies rewrite to the parts of text that are not inside
// quotes. Quoted text is what a pod wrote; a rewrite of kwatch's own
// wording (kind spelling, tense, plural) must never change it. Quotes
// are the quote runes; text without them, such as a title that was
// already written out, falls back to its double quotes.
func outsideQuotes(text string, rewrite func(string) string) string {
	if !strings.Contains(text, quoteOpen) {
		return outsideDoubleQuotes(text, rewrite)
	}
	var b strings.Builder
	for _, seg := range segmentsOf(text) {
		if seg.quoted {
			b.WriteString(markQuote(seg.text))
			continue
		}
		b.WriteString(outsideDoubleQuotes(seg.text, rewrite))
	}
	return b.String()
}

// outsideDoubleQuotes is outsideQuotes for text whose quotations are
// written with double quotes.
func outsideDoubleQuotes(text string, rewrite func(string) string) string {
	if !strings.Contains(text, `"`) {
		return rewrite(text)
	}
	parts := strings.Split(text, `"`)
	for i := 0; i < len(parts); i += 2 {
		parts[i] = rewrite(parts[i])
	}
	return strings.Join(parts, `"`)
}

// cutOutsideQuotes cuts text at the first sep that is not inside a
// quotation. Without a sep outside the quotes it reports false.
func cutOutsideQuotes(text, sep string) (string, string, bool) {
	open, end := quoteOpen, quoteClose
	if !strings.Contains(text, quoteOpen) {
		open, end = `"`, `"`
	}
	inside := false
	for i := 0; i < len(text); {
		switch {
		case !inside && strings.HasPrefix(text[i:], open):
			inside = true
			i += len(open)
		case inside && strings.HasPrefix(text[i:], end):
			inside = false
			i += len(end)
		case !inside && strings.HasPrefix(text[i:], sep):
			return text[:i], text[i+len(sep):], true
		default:
			i++
		}
	}
	return text, "", false
}

// markDoubleQuotes turns the double-quoted parts of detector text into
// quotations. Detectors write what a pod or an event said between double
// quotes. One quotation that starts after a colon and runs to the end of
// the text may hold quotes of its own; otherwise the quotes are paired in
// order, and an odd count leaves the text as it is.
func markDoubleQuotes(text string) string {
	text = dropQuoteRunes(text)
	first, last := strings.Index(text, `"`), strings.LastIndex(text, `"`)
	if first < 0 {
		return text
	}
	if first != last && last == len(text)-1 &&
		strings.HasSuffix(text[:first], ": ") {
		return text[:first] + markQuote(text[first+1:last])
	}
	if strings.Count(text, `"`)%2 != 0 {
		return text
	}
	parts := strings.Split(text, `"`)
	var b strings.Builder
	for i, part := range parts {
		b.WriteString(part)
		switch {
		case i == len(parts)-1:
		case i%2 == 0:
			b.WriteString(quoteOpen)
		default:
			b.WriteString(quoteClose)
		}
	}
	return b.String()
}

// markedTitle is the headline of msg without its marker, with the quote
// runes put back around what a pod wrote. A list that builds its own
// lines from titles (a digest, a roll-up) starts from this, so a quotation
// is still found by its ends.
func markedTitle(msg notification.Message) string {
	if len(msg.Doc) == 0 {
		return msg.Title
	}
	var b strings.Builder
	for i, span := range msg.Doc[0].Spans {
		text := span.Text
		if i == 0 {
			text = strings.TrimPrefix(text, msg.Marker+" ")
		}
		if span.Style == notification.Code {
			text = markQuote(text)
		}
		b.WriteString(text)
	}
	if plainQuotes(b.String()) != msg.Title {
		return msg.Title
	}
	return b.String()
}
