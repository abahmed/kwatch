package compose

import (
	"testing"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// Only a whole kind word followed by a name is respelled, in any case.
func TestRespellOnlyRewritesKindBeforeAName(t *testing.T) {
	names := map[inventory.Kind]string{
		kube.KindFor("DatadogAgent"): "DatadogAgent"}
	cases := map[string]string{
		"datadogagent datadog is failing.": "DatadogAgent datadog is failing.",
		"Datadogagent datadog is failing.": "DatadogAgent datadog is failing.",
		"the datadogagents datadog":        "the datadogagents datadog",
		"nothing fails in datadogagent":    "nothing fails in datadogagent",
	}
	for in, want := range cases {
		got := respell([]sentence{{part: partLead, text: in}}, names)
		if got[0].text != want {
			t.Errorf("respell(%q) = %q; want %q", in, got[0].text, want)
		}
	}
}

// Text a pod wrote, in quotes, is quoted as it was: its kind words are
// not respelled. The same words outside the quotes are.
func TestRespellSkipsQuotedText(t *testing.T) {
	names := map[inventory.Kind]string{
		kube.KindFor("DatadogAgent"): "DatadogAgent"}
	in := `datadogagent a fails with "datadogagent b not found" today`
	want := `DatadogAgent a fails with "datadogagent b not found" today`

	got := respell([]sentence{{part: partLead, text: in}}, names)

	if got[0].text != want {
		t.Fatalf("respell = %q; want %q", got[0].text, want)
	}
}
