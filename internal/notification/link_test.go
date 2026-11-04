package notification

import (
	"strings"
	"testing"
)

const linkAddress = "https://kwatch.example/issues/p-1"

// renderLinkLine renders one block that is a single link span.
func renderLinkLine(d Dialect, label, address string) string {
	return RenderBlocks([]Block{{Kind: Para,
		Spans: []Span{LinkSpan(label, address)}}}, d)
}

func TestDialectLinkWritesEachProviderMarkup(t *testing.T) {
	label := "Open in dashboard"
	cases := map[string]struct {
		d    Dialect
		want string
	}{
		"plain": {PlainDialect(), label + ": " + linkAddress},
		"slack": {SlackDialect(), "<" + linkAddress + "|" + label + ">"},
		"markdown": {MarkdownDialect(nil, "\n"),
			"[" + label + "](" + linkAddress + ")"},
		"feishu": {FeishuDialect(),
			"[" + label + "](" + linkAddress + ")"},
		"dingtalk": {DingTalkDialect(),
			"[" + label + "](" + linkAddress + ")"},
		"googlechat": {GoogleChatDialect(),
			"<" + linkAddress + "|" + label + ">"},
		"html": {HTMLDialect(nil, true),
			`<a href="` + linkAddress + `">` + label + "</a>"},
		"jira": {JiraDialect(), "[" + label + "|" + linkAddress + "]"},
	}
	for name, c := range cases {
		if got := renderLinkLine(c.d, label, linkAddress); got != c.want {
			t.Errorf("%s = %q, want %q", name, got, c.want)
		}
	}
}

func TestDialectLinkEscapesLabel(t *testing.T) {
	label := "a|b > c ] d ) e & f"
	cases := map[string]struct {
		d    Dialect
		want string
	}{
		"slack": {SlackDialect(), "<" + linkAddress +
			"|a|b &gt; c ] d ) e &amp; f>"},
		"markdown": {MarkdownDialect(nil, "\n"), `[a\|b \> c \] d \) e & f](` +
			linkAddress + ")"},
		"html": {HTMLDialect(nil, false), `<a href="` + linkAddress +
			`">a|b &gt; c ] d ) e &amp; f</a>`},
		"jira": {JiraDialect(), `[a\|b > c \] d ) e & f|` +
			linkAddress + "]"},
		"googlechat": {GoogleChatDialect(), label + ": " + linkAddress},
	}
	for name, c := range cases {
		if got := renderLinkLine(c.d, label, linkAddress); got != c.want {
			t.Errorf("%s = %q, want %q", name, got, c.want)
		}
	}
}

func TestDialectLinkEncodesAddress(t *testing.T) {
	address := "https://k.example/a b/(x)|y>z?q=1&r=[2]#f"
	encoded := "https://k.example/a%20b/%28x%29%7Cy%3Ez?q=1&r=%5B2%5D#f"
	cases := map[string]struct {
		d    Dialect
		want string
	}{
		"slack": {SlackDialect(), "<" +
			strings.ReplaceAll(encoded, "&", "&amp;") + "|go>"},
		"markdown": {MarkdownDialect(nil, "\n"),
			"[go](" + encoded + ")"},
		"html": {HTMLDialect(nil, false), `<a href="` +
			strings.ReplaceAll(encoded, "&", "&amp;") + `">go</a>`},
		"jira":       {JiraDialect(), "[go|" + encoded + "]"},
		"googlechat": {GoogleChatDialect(), "<" + encoded + "|go>"},
	}
	for name, c := range cases {
		if got := renderLinkLine(c.d, "go", address); got != c.want {
			t.Errorf("%s = %q, want %q", name, got, c.want)
		}
	}
}

func TestDialectLinkKeepsEncodedBytesAndIPv6Host(t *testing.T) {
	for _, address := range []string{
		"https://k.example/issues/a%2Fb",
		"http://[::1]:8080/issues",
	} {
		got := renderLinkLine(MarkdownDialect(nil, "\n"), "go", address)
		if want := "[go](" + address + ")"; got != want {
			t.Errorf("%q = %q, want %q", address, got, want)
		}
	}
}

func TestDialectLinkWritesOtherSchemesAsText(t *testing.T) {
	for _, address := range []string{
		"javascript:alert(1)", "ftp://k.example/x", "/issues/p-1",
		"https://", "mailto:a@k.example", "https://k<x>.example/",
	} {
		for name, d := range map[string]Dialect{
			"slack": SlackDialect(), "markdown": MarkdownDialect(nil, "\n"),
			"html": HTMLDialect(nil, false), "jira": JiraDialect(),
			"googlechat": GoogleChatDialect(),
		} {
			got := renderLinkLine(d, "go", address)
			want := d.Text(d.neutral("go: " + address))
			if got != want {
				t.Errorf("%s %q = %q, want text %q", name, address, got,
					want)
			}
		}
	}
}

func TestDialectLinkWithoutLabelIsTheAddress(t *testing.T) {
	got := renderLinkLine(SlackDialect(), " ", linkAddress)
	if got != linkAddress {
		t.Fatalf("got %q, want the bare address", got)
	}
}

func TestBlockTextReadsLinkAsLabelAndAddress(t *testing.T) {
	cases := []struct {
		span Span
		want string
	}{
		{LinkSpan("Open", linkAddress), "Open: " + linkAddress},
		{LinkSpan("Open:", linkAddress), "Open: " + linkAddress},
		{LinkSpan("", linkAddress), linkAddress},
		{Span{Text: "plain", URL: linkAddress}, "plain"},
	}
	for _, c := range cases {
		if got := (Block{Spans: []Span{c.span}}).Text(); got != c.want {
			t.Errorf("Text(%+v) = %q, want %q", c.span, got, c.want)
		}
	}
}

func TestDialectLinkNeutralizesLabelNotAddress(t *testing.T) {
	address := "https://k.example/@here"
	got := renderLinkLine(MarkdownDialect(nil, "\n"), "@here", address)
	if strings.Contains(got, "[@here]") {
		t.Fatalf("label mention survived: %q", got)
	}
	if !strings.HasSuffix(got, "("+address+")") {
		t.Fatalf("address changed: %q", got)
	}
}

// A bounded render drops a link line whole: it never leaves part of the
// markup behind.
func TestRenderWithinDropsLinkLineWhole(t *testing.T) {
	blocks := []Block{
		{Kind: Para, Spans: []Span{{Text: strings.Repeat("x", 60)}}},
		{Kind: Para, Spans: []Span{LinkSpan("Open in dashboard",
			linkAddress)}},
	}
	for name, d := range map[string]Dialect{
		"slack": SlackDialect(), "markdown": MarkdownDialect(nil, "\n"),
		"html": HTMLDialect(nil, false), "jira": JiraDialect(),
	} {
		full := RenderBlocks(blocks, d)
		for limit := 61; limit < len(full); limit++ {
			got := RenderBlocksWithin(blocks, d, limit)
			if strings.Contains(got, "kwatch.example") ||
				strings.Contains(got, "Open in") {
				t.Fatalf("%s limit %d kept part of the link: %q",
					name, limit, got)
			}
		}
		if got := RenderBlocksWithin(blocks, d, len(full)); got != full {
			t.Fatalf("%s at its own size = %q, want %q", name, got, full)
		}
	}
}
