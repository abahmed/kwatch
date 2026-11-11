package notification

import (
	"strings"
	"unicode/utf8"
)

// closingFence ends a code block the cut left open.
const closingFence = "\n```"

// Truncate cuts text to at most limit bytes, ending in an ellipsis. It
// cuts between words when a space is near the limit (see wordCut), and
// on a rune boundary otherwise. Cutting is deterministic, so a retried
// delivery sends the same bytes.
//
// The cut keeps Markdown well formed: a code fence the cut leaves open
// is closed, so a provider never renders the rest of a message as code.
// A limit of zero or less leaves no room, so the result is empty: a
// caller that means "no limit" must not call Truncate at all.
//
// Plain text is never searched for tags; use TruncateHTML for markup.
func Truncate(text string, limit int) string {
	return truncate(text, limit, false)
}

// TruncateHTML is Truncate for HTML text: it also drops a tag the cut
// splits in half ("<b" or "</co"), so no broken tag is sent. It does
// not close an element the cut leaves open ("<b>bold…" stays so).
func TruncateHTML(text string, limit int) string {
	return truncate(text, limit, true)
}

func truncate(text string, limit int, markup bool) string {
	if limit <= 0 {
		return ""
	}
	if len(text) <= limit {
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
	cut := wordCut(text, room)
	head := wholeMarkup(text, cut, markup)
	if strings.Count(head, "```")%2 == 1 {
		return head + ellipsis + closingFence
	}
	return head + ellipsis
}

// maxWordBackoff bounds how far wordCut moves the cut back to a space:
// at most a quarter of the room and at most maxWordBytes, about the
// longest plain word. A longer run without a space (a path, a token) is
// cut at the limit instead.
const (
	maxWordBackoff = 4
	maxWordBytes   = 32
)

// wordCut is where a cut of text at room bytes ends: before the word
// the limit falls in, when a space is close enough, so a quote never
// ends mid-word; otherwise on the rune boundary at or before room.
func wordCut(text string, room int) int {
	cut := room
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	if cut == 0 {
		return 0
	}
	if isSpace(text[cut]) || isSpace(text[cut-1]) {
		return trimSpaceEnd(text, cut)
	}
	floor := cut - min(room/maxWordBackoff, maxWordBytes)
	for i := cut - 1; i > floor && i > 0; i-- {
		if isSpace(text[i]) {
			return trimSpaceEnd(text, i)
		}
	}
	return cut
}

// trimSpaceEnd moves cut back over the spaces before it, so the
// ellipsis follows the last word.
func trimSpaceEnd(text string, cut int) int {
	for cut > 1 && isSpace(text[cut-1]) {
		cut--
	}
	return cut
}

func isSpace(b byte) bool { return b == ' ' || b == '\n' || b == '\t' }

// wholeMarkup is text[:cut] without a fence marker, or, for HTML, a tag,
// cut in half.
func wholeMarkup(text string, cut int, html bool) string {
	head := text[:cut]
	if cut < len(text) && text[cut] == '`' {
		// The cut is inside a run of backticks, perhaps a fence.
		head = strings.TrimRight(head, "`")
	}
	if !html {
		return head
	}
	if open := strings.LastIndex(head, "<"); open >= 0 &&
		!strings.Contains(head[open:], ">") && open+1 < len(head) &&
		isTagStart(head[open+1]) && looksLikeTag(head[open:]) {
		head = head[:open]
	}
	return head
}

// Longest tail of a "<" with no ">" still taken for a tag cut in half: a
// bare tag, and one with quoted attributes such as a link. A "<" farther
// back is text, and dropping from it would lose the rest of the message.
const (
	maxTagLen     = 64
	maxTagAttrLen = 512
)

// looksLikeTag reports whether tail, which starts at a "<" and has no
// ">", can be a tag the cut stopped in.
func looksLikeTag(tail string) bool {
	if len(tail) <= maxTagLen {
		return true
	}
	return len(tail) <= maxTagAttrLen &&
		(strings.Contains(tail, `="`) || strings.Contains(tail, "='"))
}

// isTagStart reports whether b can follow the "<" of a tag.
func isTagStart(b byte) bool {
	return b == '/' || b == '!' || (b >= 'a' && b <= 'z') ||
		(b >= 'A' && b <= 'Z')
}
