package notification

import (
	"strings"
	"testing"
)

func TestFeishuDialectBreaksAtTagsAndEscapesCode(t *testing.T) {
	m := sample()
	m.Doc[1].Spans[1].Text = "boom *x* <at id=all></at> @channel"
	got := m.Render(FeishuDialect())
	if strings.Contains(got, "<at") || strings.Contains(got, "@channel") {
		t.Fatalf("live mention survives: %q", got)
	}
	if !strings.Contains(got, "`boom *x* ") {
		t.Fatalf("code span is not an inline code span: %q", got)
	}
}

func TestDingTalkDialectHasNoBackticksOrAt(t *testing.T) {
	m := sample()
	m.Doc[1].Spans[1].Text = "boom *x* @all @13800000000"
	got := m.Render(DingTalkDialect())
	if strings.Contains(got, "`") || strings.Contains(got, "@all") ||
		strings.Contains(got, "@1") {
		t.Fatalf("DingTalk text keeps code or mention: %q", got)
	}
	if !strings.Contains(got, `boom \*x\*`) {
		t.Fatalf("code span text is not escaped: %q", got)
	}
}

func TestDingTalkFenceIsAQuote(t *testing.T) {
	got := DingTalkDialect().Fence("a *b*\nc")
	if got != "> a \\*b\\*\n> c" {
		t.Fatalf("fence = %q", got)
	}
}

// An e-mail address quoted from a log keeps its @; only an @ that starts
// a word is a DingTalk mention.
func TestDingTalkKeepsEmailAddresses(t *testing.T) {
	got := NeutralizeDingTalkMentions("mail user@example.com, ping @1380 " +
		"and (@bob) @a@b")
	want := "mail user@example.com, ping @​1380 and (@​bob) " +
		"@​a@b"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
