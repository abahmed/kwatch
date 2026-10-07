package compose

import (
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

const readinessFlapLine = "Readiness probe failed: Get " +
	"\"http://10.0.1.5:8080/ready\": context deadline exceeded"

func readinessFlapFacts(evidence ...detection.Evidence) caseFacts {
	api := inventory.CoreID(kube.KindDeployment, "shop", "api")
	member := detection.Finding{Entity: api,
		Reason: reasons.ReadinessFlapping, Evidence: evidence}
	return caseFacts{p: incident.Incident{Root: api},
		members: []detection.Finding{member}}
}

func TestFlappingNamesTheServiceAndQuotesTheProbe(t *testing.T) {
	got := readinessFlapSentences(readinessFlapFacts(
		detection.Evidence{Label: "service", Value: "api"},
		detection.Evidence{Label: "readiness probe",
			Value: readinessFlapLine}))

	if len(got) != 1 {
		t.Fatalf("got %d sentences, want 1", len(got))
	}
	for _, want := range []string{"Service api's endpoints keep changing",
		"Quoted readiness failure: \"" + readinessFlapLine[:20]} {
		if !strings.Contains(got[0].text, want) {
			t.Errorf("%q does not contain %q", got[0].text, want)
		}
	}
}

func TestFlappingWithoutEvidenceSaysNothing(t *testing.T) {
	if got := readinessFlapSentences(readinessFlapFacts()); len(got) != 0 {
		t.Fatalf("got %v, want no sentences", got)
	}
}
