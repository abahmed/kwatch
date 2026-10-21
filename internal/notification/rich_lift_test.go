package notification

import (
	"strings"
	"testing"
)

func quoteLine(quote string) Message {
	return Message{Doc: []Block{
		{Kind: Para, Spans: []Span{{Text: "🔴 api is failing."}}},
		{Kind: Para, Spans: []Span{{Text: "It logs "},
			{Text: quote, Style: Code}, {Text: ". It restarts."}}},
	}}
}

// Jira cannot escape a brace inside {{ }}, so a quote that holds one is
// written as a block instead of losing its braces.
func TestJiraWritesAQuoteWithBracesAsABlock(t *testing.T) {
	quote := `{"level":"error","msg":"bad {x}"}`

	got := quoteLine(quote).Render(JiraDialect())

	want := "🔴 api is failing.\nIt logs\n{noformat}\n" + quote +
		"\n{noformat}\nIt restarts."
	if got != want {
		t.Fatalf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestJiraKeepsASpanWithoutBracesInline(t *testing.T) {
	got := quoteLine("boom *x*").Render(JiraDialect())

	if !strings.Contains(got, `It logs {{boom \*x\*}}. It restarts.`) {
		t.Fatalf("span lifted without need:\n%s", got)
	}
}

// Dialects that can write the text inline are not changed.
func TestOtherDialectsKeepBraceQuotesInline(t *testing.T) {
	quote := `{"level":"error"}`
	for name, d := range map[string]Dialect{"slack": SlackDialect(),
		"markdown": MarkdownDialect(nil, "\n\n"),
		"html":     HTMLDialect(nil, false), "plain": PlainDialect()} {
		got := quoteLine(quote).Render(d)
		if strings.Contains(got, "\nIt logs\n") ||
			!strings.Contains(got, "It logs") ||
			!strings.Contains(got, strings.ReplaceAll(quote, `"`,
				map[bool]string{true: "&#34;", false: `"`}[name == "html"])) {
			t.Errorf("%s changed:\n%s", name, got)
		}
	}
}

func TestJiraBraceQuoteInABulletKeepsItsWords(t *testing.T) {
	m := Message{Doc: []Block{{Kind: Bullet, Spans: []Span{
		{Text: "pod a said "}, {Text: "{x}", Style: Code}}}}}

	got := m.Render(JiraDialect())

	if got != "* pod a said\n{noformat}\n{x}\n{noformat}" {
		t.Fatalf("got %q", got)
	}
}
