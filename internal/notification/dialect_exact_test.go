package notification

import (
	"strings"
	"testing"
)

// A code span shows what a pod wrote: its spacing and its backticks are
// part of it.
func TestMarkdownCodeSpanKeepsSpacesAndBackticks(t *testing.T) {
	cases := map[string]string{
		"a  b":      "`a  b`",
		"a\tb":      "`a\tb`",
		" lead":     "`  lead `",
		"trail ":    "` trail  `",
		"`tick`":    "`` `tick` ``",
		"say `hi`!": "``say `hi`!``",
		"two\nline": "`two line`",
	}
	for in, want := range cases {
		got := MarkdownDialect(nil, "\n").Code(in)
		if got != want {
			t.Errorf("Code(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestChatCodeSpansKeepSpaces(t *testing.T) {
	text := "dial tcp:  lookup   db"
	for name, d := range map[string]Dialect{
		"slack": SlackDialect(), "googlechat": GoogleChatDialect(),
		"plain": PlainDialect(),
		"html":  HTMLDialect(nil, false), "jira": JiraDialect()} {
		if got := d.Code(text); !strings.Contains(got, text) {
			t.Errorf("%s dropped spaces: %q", name, got)
		}
	}
}

func TestSlackSmallPrintKeepsUnderscores(t *testing.T) {
	blocks := []Block{{Kind: Small, Spans: []Span{{
		Text: "my_service_x and __init__ keep their underscores"}}}}
	for name, d := range map[string]Dialect{
		"slack": SlackDialect(), "googlechat": GoogleChatDialect()} {
		got := RenderBlocks(blocks, d)
		if !strings.Contains(got, "my_service_x and __init__") {
			t.Errorf("%s deleted an underscore: %q", name, got)
		}
	}
}

func TestSlackBoldNameKeepsStars(t *testing.T) {
	blocks := []Block{{Kind: Para, Spans: []Span{{
		Text: "a*b", Style: Bold}}}}
	if got := RenderBlocks(blocks, SlackDialect()); !strings.Contains(got,
		"a*b") {
		t.Errorf("star deleted: %q", got)
	}
}

func TestChatCodeBlockCarriesBackticks(t *testing.T) {
	body := "ran `ls` and `cat`  now"
	blocks := []Block{{Kind: CodeBlock, Spans: []Span{{Text: body}}}}
	for name, d := range map[string]Dialect{
		"slack": SlackDialect(), "googlechat": GoogleChatDialect(),
		"markdown": MarkdownDialect(nil, "\n"),
		"html":     HTMLDialect(nil, false)} {
		got := RenderBlocks(blocks, d)
		if strings.Contains(got, "ˋ") ||
			!strings.Contains(got, strings.ReplaceAll(body, "<", "&lt;")) {
			t.Errorf("%s changed the block: %q", name, got)
		}
	}
}
