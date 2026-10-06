package notification

import (
	"strings"
	"testing"
)

// sample is one incident: headline, evidence, a quoted pod error, the
// affected line and the command.
func sample() Message {
	return Message{
		Note: "🔴 payments is down in shop.",
		Doc: []Block{
			{Kind: Para, Spans: []Span{{Text: "🔴 "},
				{Text: "payments", Style: Bold},
				{Text: " is down in "},
				{Text: "shop", Style: Bold}, {Text: "."}}},
			{Kind: Para, Spans: []Span{{Text: "It logged "},
				{Text: "panic: key <!channel> `x` *y* _z_ @here",
					Style: Code}, {Text: "."}}},
			{Kind: Para, Spans: []Span{{Text: "Service "},
				{Text: "web", Style: Bold},
				{Text: " can't serve traffic."}}},
			{Kind: CodeBlock, Spans: []Span{{
				Text: "kubectl logs payments -n shop"}}},
		},
	}
}

func TestRenderSlack(t *testing.T) {
	got := sample().Render(SlackDialect())
	want := "🔴 *payments* is down in *shop*.\n" +
		"It logged `panic: key &lt;!channel&gt; ˋxˋ *y* _z_ @​here`.\n" +
		"Service *web* can't serve traffic.\n" +
		"```\nkubectl logs payments -n shop\n```"
	if got != want {
		t.Fatalf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestRenderMarkdownFamily(t *testing.T) {
	got := sample().Render(MarkdownDialect(nil, "\n\n"))
	if !strings.Contains(got, "**payments**") ||
		!strings.Contains(got, "``panic: key <!channel> `x` *y* _z_ @​here``") ||
		!strings.HasSuffix(got, "```\nkubectl logs payments -n shop\n```") {
		t.Fatalf("markdown:\n%s", got)
	}
}

func TestRenderTelegramHTMLEscapes(t *testing.T) {
	got := sample().Render(HTMLDialect(nil, false))
	for _, want := range []string{
		"<b>payments</b>",
		"<code>panic: key &lt;!channel&gt; `x` *y* _z_ @​here</code>",
		"<pre>kubectl logs payments -n shop</pre>",
		"can&#39;t",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
}

func TestRenderJiraWiki(t *testing.T) {
	m := sample()
	m.Doc[1].Spans[1].Text = "bad {code} [~admin] *x*"
	got := m.Render(JiraDialect())
	for _, want := range []string{"*payments*",
		"{{bad code [~admin] *x*}}", "{noformat}\nkubectl"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "{{bad {code}") {
		t.Errorf("braces must not survive inside a code span: %s", got)
	}
}

func TestRenderPlainStripsMarkup(t *testing.T) {
	got := sample().Plain()
	if strings.ContainsAny(got, "*`<>&") && !strings.Contains(got,
		"panic: key <!channel>") {
		t.Fatalf("plain text: %q", got)
	}
	if strings.Contains(got, "**") || strings.Contains(got, "<b>") ||
		strings.Contains(got, "```") {
		t.Fatalf("markup left in plain text: %q", got)
	}
	if !strings.HasPrefix(got, "🔴 payments is down in shop.\n") {
		t.Fatalf("plain text: %q", got)
	}
}

func TestNoteTextIsPlainLinesWhenBlocksExist(t *testing.T) {
	m := sample()
	if m.NoteText() != m.Plain() {
		t.Fatal("NoteText should be the plain lines")
	}
	if m.NoteParagraph() != "🔴 payments is down in shop." {
		t.Fatalf("the paragraph is the legacy Note: %q", m.NoteParagraph())
	}
	m.Doc = nil
	if m.NoteText() != m.Note {
		t.Fatal("without blocks NoteText is the Note")
	}
}

func TestPodTextCannotInjectMarkup(t *testing.T) {
	hostile := "*b* _i_ `c` ``` [l](u) <b>h</b> @channel <@U1> {{x}} ~s~"
	m := Message{Doc: []Block{
		{Kind: Para, Spans: []Span{{Text: hostile, Style: Code}}},
		{Kind: Para, Spans: []Span{{Text: hostile, Style: Bold}}},
	}}
	slack := m.Render(SlackDialect())
	if strings.Contains(slack, "<b>") || strings.Contains(slack, "<@U1>") ||
		strings.Contains(slack, "@channel") {
		t.Errorf("slack: %s", slack)
	}
	md := m.Render(MarkdownDialect(nil, "\n"))
	lines := strings.Split(md, "\n")
	if strings.Contains(md, "@channel") ||
		strings.Contains(lines[len(lines)-1], "<b>") {
		t.Errorf("markdown: %s", md)
	}
	// A fence inside a code block never closes it early.
	block := Message{Doc: []Block{{Kind: CodeBlock,
		Spans: []Span{{Text: "a\n```\nb"}}}}}
	if got := block.Render(MarkdownDialect(nil, "\n")); !strings.HasPrefix(
		got, "````\n") || !strings.HasSuffix(got, "\n````") {
		t.Errorf("fence must be longer than the backtick run: %q", got)
	}
	if got := block.Render(SlackDialect()); strings.Count(got, "```") != 2 {
		t.Errorf("slack fence: %q", got)
	}
}

func TestRenderWithinCutsWholeBlocks(t *testing.T) {
	var doc []Block
	for i := 0; i < 20; i++ {
		doc = append(doc, Block{Kind: Bullet, Spans: []Span{
			{Text: "item", Style: Bold}, {Text: " number"}}})
	}
	m := Message{Doc: doc}
	got := m.RenderWithin(SlackDialect(), 100)
	if len(got) > 100 || !strings.HasSuffix(got, "...") {
		t.Fatalf("got %d bytes: %q", len(got), got)
	}
	for _, line := range strings.Split(got, "\n")[:3] {
		if line != "• *item* number" {
			t.Fatalf("a block was cut inside its markup: %q", line)
		}
	}
}

func TestWithMarkerSwapsBlocksAndNote(t *testing.T) {
	m := sample()
	swapped := m.WithMarker(MarkerResolved)
	if !strings.HasPrefix(swapped.Plain(), MarkerResolved+" payments") ||
		!strings.HasPrefix(swapped.Note, MarkerResolved) {
		t.Fatalf("marker not swapped: %q", swapped.Plain())
	}
	if !strings.HasPrefix(m.Plain(), MarkerPage) {
		t.Fatal("the original message was changed")
	}
}
