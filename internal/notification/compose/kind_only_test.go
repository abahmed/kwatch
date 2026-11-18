package compose

import (
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/rootcause"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

func signatureFacts(line string) caseFacts {
	return caseFacts{p: incident.Incident{Cause: &rootcause.CauseRecord{
		Root: inventory.CoreID(explain.KindFailureSignature, "", "x"),
		Rule: "shared-failure-signature", SharedError: line}}}
}

func proofFacts(proof ...rootcause.Proof) caseFacts {
	return caseFacts{p: incident.Incident{Cause: &rootcause.CauseRecord{
		Proof: proof}}}
}

func TestKindOnlyLeadSaysTheAddressIsNotNamed(t *testing.T) {
	got := kindOnlyLead(signatureFacts("RedisConnectionException: It "+
		"was not possible to connect to the redis server(s)"), "base")
	want := "base: they cannot reach their cache (Redis), and the " +
		"error does not name the address"
	if got != want {
		t.Fatalf("got %q", got)
	}
}

func TestKindOnlyLeadKeepsOtherErrors(t *testing.T) {
	for _, line := range []string{"panic: license check failed",
		"dial tcp 10.0.7.20:6379: connect: connection refused"} {
		if got := kindOnlyLead(signatureFacts(line), "base"); got != "base" {
			t.Errorf("%q: got %q", line, got)
		}
	}
}

func TestNotStatedSentencesNameTheWorkloads(t *testing.T) {
	f := proofFacts(rootcause.Proof{Code: rootcause.ProofAddressNotStated,
		Count: 2, Fields: []string{"pricing", "tracking"}})
	got := notStatedSentences(f)
	if len(got) != 1 || !strings.HasPrefix(got[0].text,
		"pricing and tracking fail the same way; their errors do not") {
		t.Fatalf("got %+v", got)
	}
	if notStatedSentences(proofFacts()) != nil {
		t.Fatal("no note, yet a sentence")
	}
}
