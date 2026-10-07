package compose

import (
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

func missingServiceFacts(
	rule, line string, proofs ...rootcause.Proof,
) caseFacts {
	service := inventory.CoreID(kube.KindService, "shop", "paymnts")
	api := inventory.CoreID(kube.KindDeployment, "shop", "api")
	crash := inventory.CoreID(kube.KindContainer, "shop", "api-1-a/app")
	member := detection.Finding{Entity: crash, Reason: "CrashLoopBackOff",
		Severity: detection.Critical}
	if line != "" {
		member.Evidence = []detection.Evidence{{
			Label: detection.EvidenceError, Value: line}}
	}
	return caseFacts{
		p: incident.Incident{Root: service,
			Impact: []inventory.EntityID{api},
			Cause: &rootcause.CauseRecord{Rule: rule, Root: service,
				Proof: proofs}},
		members: []detection.Finding{member},
	}
}

func TestMissingServiceSaysWhatIsCalledQuotesAndSuggests(t *testing.T) {
	line := "dial tcp: lookup paymnts.shop.svc.cluster.local: no such host"
	got := missingServiceSentences(missingServiceFacts(
		"missing-service-called", line, rootcause.Proof{
			Code: rootcause.ProofMissingCall, Count: 8080,
			Fields: []string{"payments"}}))

	want := "api calls paymnts:8080, but there is no Service paymnts in " +
		"shop. Quoted: \"" + line + "\". A Service named payments exists."
	if len(got) != 1 || got[0].text != want {
		t.Fatalf("got %+v\nwant %q", got, want)
	}
}

func TestMissingServiceFromConfigurationQuotesNothing(t *testing.T) {
	got := missingServiceSentences(missingServiceFacts(
		"missing-service-configured", "panic: configuration invalid"))

	if len(got) != 1 || strings.Contains(got[0].text, "Quoted") ||
		strings.Contains(got[0].text, "exists") ||
		!strings.Contains(got[0].text, "api calls paymnts, but there is no") {
		t.Fatalf("got %+v", got)
	}
}

func TestMissingServiceIgnoresOtherCauses(t *testing.T) {
	if got := missingServiceSentences(missingServiceFacts("self",
		"")); len(got) != 0 {
		t.Fatalf("got %+v", got)
	}
}
