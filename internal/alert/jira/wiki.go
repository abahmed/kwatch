package jira

import (
	"regexp"
	"strings"
)

// macroStart matches a wiki macro opener at the start of text, such as
// {code}, {color:red} or {panel:title=x}. A brace that starts JSON text,
// like {"a": 1}, does not match and is left alone.
var macroStart = regexp.MustCompile(`^\{[A-Za-z][A-Za-z0-9-]*[:}]`)

// activeLink finds the start of a bracket that Jira turns into a mention
// or link: [~accountid], [text|url] or [http://...].
var activeLink = regexp.MustCompile(`^\[[^\]\n]*(\||~|://|mailto:)`)

// literalBlocks are the macros whose content is not interpreted.
var literalBlocks = []string{"{code", "{noformat"}

// escapeWiki backslash-escapes the Jira wiki markup that acts on text:
// [~accountid] mentions and [link|url] links, {macro} openers and !image!
// embeds. The Note can carry pod-controlled text. Plain braces and
// brackets, such as JSON, stay as they are, and a complete {code} or
// {noformat} block is kept untouched because nothing inside it is active.
func escapeWiki(text string) string {
	var b strings.Builder
	for i := 0; i < len(text); {
		if end := literalBlockEnd(text[i:]); end > 0 {
			b.WriteString(text[i : i+end])
			i += end
			continue
		}
		rest := text[i:]
		switch {
		case macroStart.MatchString(rest),
			activeLink.MatchString(rest),
			isImage(rest):
			b.WriteByte('\\')
		}
		b.WriteByte(text[i])
		i++
	}
	return b.String()
}

// literalBlockEnd returns the length of the complete {code} or
// {noformat} block at the start of text, or 0 when there is none. An
// opener without its closer is not a block and gets escaped.
func literalBlockEnd(text string) int {
	for _, name := range literalBlocks {
		if !strings.HasPrefix(text, name) {
			continue
		}
		open := macroStart.FindString(text)
		if open == "" || !strings.HasPrefix(text, open) {
			continue
		}
		closer := name + "}"
		at := strings.Index(text[len(open):], closer)
		if at >= 0 {
			return len(open) + at + len(closer)
		}
	}
	return 0
}

// isImage reports whether text starts an !image! embed: a "!" followed by
// something other than a space and a second "!" on the same line.
func isImage(text string) bool {
	if len(text) < 2 || text[0] != '!' || text[1] == ' ' || text[1] == '\n' {
		return false
	}
	line, _, _ := strings.Cut(text[1:], "\n")
	return strings.Contains(line, "!")
}
