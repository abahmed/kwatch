package notification

import (
	"strings"
	"testing"
)

// pinnedDoc is a message whose proof is longer than limit and that ends
// with a pinned fix.
func pinnedDoc(proof int) []Block {
	doc := []Block{{Kind: Para, Spans: []Span{{Text: "🔴 api in shop " +
		"was killed for using more memory than its 256Mi limit."}}}}
	for i := 0; i < proof; i++ {
		doc = append(doc, Block{Kind: Bullet, Spans: []Span{{Text: "fact " +
			strings.Repeat("x", 180)}}})
	}
	return append(doc,
		Block{Kind: Para, Pinned: true,
			Spans: []Span{{Text: "Raise the limit (changes the cluster):"}}},
		Block{Kind: CodeBlock, Pinned: true, Spans: []Span{{
			Text: "kubectl set resources deployment/api -n shop " +
				"--limits=memory=384Mi"}}})
}

func TestRenderBlocksWithinKeepsPinnedFix(t *testing.T) {
	d := MarkdownDialect(nil, "\n")
	for _, limit := range []int{600, 2000, 4096} {
		out := RenderBlocksWithin(pinnedDoc(40), d, limit)
		if len(out) > limit {
			t.Fatalf("limit %d: rendered %d bytes", limit, len(out))
		}
		if !strings.Contains(out, "--limits=memory=384Mi") ||
			!strings.Contains(out, "256Mi limit") {
			t.Fatalf("limit %d: cause or fix was cut:\n%s", limit, out)
		}
	}
}

func TestRenderBlocksWithinUnpinnedIsUnchanged(t *testing.T) {
	d := MarkdownDialect(nil, "\n")
	doc := pinnedDoc(40)
	for i := range doc {
		doc[i].Pinned = false
	}
	if RenderBlocksWithin(doc, d, 600) != renderWithin(doc, d, 600) {
		t.Fatal("a message without a pinned fix must render as before")
	}
}
