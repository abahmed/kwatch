package compose

import (
	"math/rand"
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/notification"
	"github.com/abahmed/kwatch/internal/rootcause"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// quotePieces are the building blocks of hostile cluster text: double
// quotes, backticks, runs of spaces, markup and a final full stop.
var quotePieces = []string{
	`"`, "`", "  ", " ", "a", "db", "x=1", "*", "_", "<b>", "@channel",
	"[x](y)", ".", ";", ":", "{", "}", `\"`, "10s", "2m0s", "é",
}

func hostileQuote(rng *rand.Rand) string {
	var b strings.Builder
	for i := 0; i < 1+rng.Intn(14); i++ {
		b.WriteString(quotePieces[rng.Intn(len(quotePieces))])
	}
	return strings.TrimSpace(b.String())
}

// quotePayloads are the texts that a rich line carries as code, in the
// order they appear.
func quotePayloads(blocks []notification.Block) []string {
	var out []string
	for _, b := range blocks {
		if b.Kind == notification.CodeBlock {
			out = append(out, b.Text())
			continue
		}
		for _, s := range b.Spans {
			if s.Style == notification.Code {
				out = append(out, s.Text)
			}
		}
	}
	return out
}

func TestQuotedTextStaysByteExactInParagraphs(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	for i := 0; i < 3000; i++ {
		payload := hostileQuote(rng)
		if payload == "" || strings.HasSuffix(payload, "\\") {
			continue
		}
		text := "It fails with " + quoted(payload) + ". Its last run " +
			"lasted 10s."
		got := quotePayloads(paragraphs(text))
		if len(got) != 1 || got[0] != payload {
			t.Fatalf("payload %q came out as %q", payload, got)
		}
	}
}

func TestQuotedTextKeepsItsFinalFullStop(t *testing.T) {
	text := plainQuotes(quoted("connection refused."))
	if text != `"connection refused."` {
		t.Fatalf("quote lost its full stop: %q", text)
	}
}

func TestQuotedInlineSpanIsExactOrPlainText(t *testing.T) {
	rng := rand.New(rand.NewSource(11))
	for i := 0; i < 3000; i++ {
		payload := hostileQuote(rng)
		if payload == "" {
			continue
		}
		spans := podSpans("It fails with " + quoted(payload) + ".")
		var joined strings.Builder
		for _, s := range spans {
			if s.Style == notification.Code && s.Text != payload {
				t.Fatalf("code span %q is not payload %q", s.Text, payload)
			}
			joined.WriteString(s.Text)
		}
		if !strings.Contains(joined.String(), payload) {
			t.Fatalf("payload %q lost in %q", payload, joined.String())
		}
	}
}

func TestQuoteMarksNeverReachTheNote(t *testing.T) {
	msg := notification.Message{}
	fill(&msg, "🟠", []sentence{
		{part: partLead, text: "web is failing in shop."},
		{part: partProof, text: "It fails with " +
			quoted(`dial tcp: lookup "db" failed.`) + "."},
	})
	for name, text := range map[string]string{"note": msg.Note,
		"short": msg.Short, "title": msg.Title,
		"lines": strings.Join(msg.Lines, "\n")} {
		if strings.ContainsAny(text, quoteOpen+quoteClose) {
			t.Errorf("%s holds a quote mark: %q", name, text)
		}
	}
	want := `It fails with "dial tcp: lookup "db" failed."`
	if !strings.Contains(msg.Note, want) || msg.Lines[0] != want {
		t.Errorf("quote changed: note %q lines %q", msg.Note, msg.Lines)
	}
	if got := quotePayloads(msg.Doc); len(got) != 1 ||
		got[0] != `dial tcp: lookup "db" failed.` {
		t.Errorf("rich quote changed: %q", got)
	}
}

func TestClipDropsTheQuoteMarks(t *testing.T) {
	got := clip("bad \x1e text \x1f here")
	if strings.ContainsAny(got, quoteOpen+quoteClose) {
		t.Fatalf("clip kept a quote mark: %q", got)
	}
}

// A failure signature holds only normalised words; they are never shown
// as if a pod had written them.
func TestFailureSignatureNameQuotesNothing(t *testing.T) {
	id := inventory.CoreID(explain.KindFailureSignature, "",
		"CrashLoop panic: license key rejected")
	got := shortName(id)
	if strings.ContainsAny(got, "\"\x1e\x1f") ||
		strings.Contains(got, "license") {
		t.Fatalf("signature name shows normalised text: %q", got)
	}
}

// A quotation that ends with a full stop ends the sentence: kwatch does
// not add a second one after the closing quote.
func TestSentenceDoesNotRepeatTheStopOfAQuotation(t *testing.T) {
	var msg notification.Message
	fill(&msg, "🟠", []sentence{
		{part: partLead, text: "web is failing."},
		{part: partProof, text: "It fails with " + quoted("boom.") + "."},
		{part: partConsequence, text: "It says " + quoted("slow") + "."},
	})
	if want := `🟠 web is failing. It fails with "boom." It says "slow".`; msg.
		Note != want {
		t.Fatalf("note = %q, want %q", msg.Note, want)
	}
	if got := quotePayloads(msg.Doc); len(got) != 2 || got[0] != "boom." {
		t.Fatalf("payloads = %q", got)
	}
}

// A signature root is named by the error a pod wrote, as written, not by
// the normalised words the entity holds.
func TestSharedErrorIsQuotedAsTheOriginalText(t *testing.T) {
	cart := inventory.CoreID(kube.KindDeployment, "shop", "cart")
	sig := inventory.CoreID(explain.KindFailureSignature, "",
		"CrashLoop panic: license key rejected")
	cause := rootcause.CauseRecord{Rule: "shared-failure-signature", Root: sig,
		SharedError: `panic: License key "K-42" rejected (code 77)`}

	got := plainQuotes(causePhrase(&cause, cart))

	want := `the error "panic: License key "K-42" rejected (code 77)" ` +
		"is what several workloads fail with"
	if got != want {
		t.Fatalf("causePhrase = %q, want %q", got, want)
	}
	cause.SharedError = ""
	if got := causePhrase(&cause, cart); strings.Contains(got, "license") {
		t.Fatalf("without the original the name is generic: %q", got)
	}
}
