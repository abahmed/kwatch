package notification

import (
	"strings"
	"unicode/utf8"
)

// closingFence ends a code block the cut left open.
const closingFence = "\n```"

// Truncate cuts text to at most limit bytes on a rune boundary, ending in
// an ellipsis. Cutting is deterministic, so a retried delivery sends the
// same bytes.
//
// The cut keeps Markdown and HTML well formed: a code fence the cut
// leaves open is closed, and a tag cut in half is dropped, so a provider
// never renders the rest of a message as code or rejects it.
func Truncate(text string, limit int) string {
	if limit <= 0 || len(text) <= limit {
		return text
	}
	const ellipsis = "…"
	room := limit - len(ellipsis)
	if strings.Contains(text, "```") {
		room -= len(closingFence)
	}
	if room <= 0 {
		return ""
	}
	cut := room
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	head := wholeMarkup(text, cut)
	if strings.Count(head, "```")%2 == 1 {
		return head + ellipsis + closingFence
	}
	return head + ellipsis
}

// wholeMarkup is text[:cut] without a fence marker or tag cut in half.
func wholeMarkup(text string, cut int) string {
	head := text[:cut]
	if cut < len(text) && text[cut] == '`' {
		// The cut is inside a run of backticks, perhaps a fence.
		head = strings.TrimRight(head, "`")
	}
	if open := strings.LastIndex(head, "<"); open >= 0 &&
		!strings.Contains(head[open:], ">") && open+1 < len(head) &&
		isTagStart(head[open+1]) {
		head = head[:open]
	}
	return head
}

// isTagStart reports whether b can follow the "<" of a tag.
func isTagStart(b byte) bool {
	return b == '/' || b == '!' || (b >= 'a' && b <= 'z') ||
		(b >= 'A' && b <= 'Z')
}
