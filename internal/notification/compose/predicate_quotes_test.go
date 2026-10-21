package compose

import (
	"testing"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// What a pod or an event said is quoted byte for byte: durations are not
// spoken and a semicolon does not end it.
func TestPredicateKeepsQuotedTextByteExact(t *testing.T) {
	svc := inventory.CoreID(kube.KindService, "shop", "web")
	cases := []struct{ summary, want string }{
		{`has been waiting 2 min for a load balancer: "context deadline ` +
			`exceeded after 30s; retrying in 5m"`,
			`has been waiting 2 min for a load balancer: ` +
				`"context deadline exceeded after 30s; retrying in 5m"`},
		{`reports "Liveness probe failed: timeout 1s; last seen 72h0m0s ago"`,
			`reports "Liveness probe failed: timeout 1s; last seen ` +
				`72h0m0s ago"`},
		{`reports "failed after 2m0s" for 72h0m0s`,
			`reports "failed after 2m0s" for three days`},
		{`TLS certificate expires in 72h0m0s; renew it`,
			`has a TLS certificate that expires in three days`},
		{`reports "a; b"; this is commentary`, `reports "a; b"`},
		{`is failing: Kubernetes reported FailedMount: ` +
			`"MountVolume.SetUp failed for volume "x": timed out after 2m0s"`,
			`is failing: Kubernetes reported FailedMount: ` +
				`"MountVolume.SetUp failed for volume "x": timed out ` +
				`after 2m0s"`},
	}
	for _, c := range cases {
		got := plainQuotes(predicate(svc, c.summary))
		if got != c.want {
			t.Errorf("summary %q\n got %q\nwant %q", c.summary, got, c.want)
		}
	}
}

func TestPredicateQuotesAreMarkedForRichLines(t *testing.T) {
	svc := inventory.CoreID(kube.KindService, "shop", "web")
	got := predicate(svc, `reports "a; b"; commentary`)
	blocks := paragraphs("web " + got + ".")
	if payloads := quotePayloads(blocks); len(payloads) != 1 ||
		payloads[0] != "a; b" {
		t.Fatalf("rich quote = %q", payloads)
	}
}

func TestBeforeColonDoesNotCutInsideAQuote(t *testing.T) {
	text := "keeps failing with " + markQuote("a: b") + ": more"
	if got := beforeColon(text); got != "keeps failing with "+
		markQuote("a: b") {
		t.Fatalf("beforeColon = %q", got)
	}
}
