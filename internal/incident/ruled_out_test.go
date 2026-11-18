package incident

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

func ruledOutIncident(checked ...string) Incident {
	return Incident{
		Root:    inventory.CoreID(kube.KindDeployment, "shop", "api"),
		Checked: checked,
	}
}

func TestRuledOutListsExactlyTheCheckedKinds(t *testing.T) {
	p := ruledOutIncident(string(kube.KindImage), string(kube.KindNode),
		string(kube.KindSecret))

	got := RuledOutOf(p, nil,
		"nothing in shop was changed in the last 30 minutes")

	assert.Equal(t, []string{"its node is healthy",
		"its image shows no pull errors and did not change",
		"its Secrets are there and did not change",
		"nothing in shop was changed in the last 30 minutes"}, got.Fine)
	assert.False(t, got.Also)
	assert.Empty(t, got.Unchecked)
}

func TestRuledOutSaysNothingWhenNothingWasChecked(t *testing.T) {
	got := RuledOutOf(ruledOutIncident(), nil, "")

	assert.True(t, got.Empty())
}

func TestRuledOutForAPossibleCauseUsesNouns(t *testing.T) {
	p := ruledOutIncident(string(kube.KindConfigMap),
		string(kube.KindSecret), string(kube.KindNode))
	p.Cause = &rootcause.CauseRecord{Score: 0.9,
		Rival: &rootcause.CauseRecord{Score: 0.88}}

	got := RuledOutOf(p, nil, "a quiet history is not said here")

	assert.True(t, got.Also)
	assert.Equal(t, []string{"its node", "its ConfigMaps and Secrets"},
		got.Fine)
}

func TestRuledOutSaysNothingForAConfidentCause(t *testing.T) {
	p := ruledOutIncident(string(kube.KindNode))
	p.Cause = &rootcause.CauseRecord{Score: 0.95}

	assert.True(t, RuledOutOf(p, nil, "").Empty())
}

func TestRuledOutSaysWhatCouldNotBeRead(t *testing.T) {
	report := []Section{
		{Check: CheckLogs, Status: SectionDenied, Reason: ReasonRBAC},
		{Check: CheckLogs, Status: SectionDenied, Reason: ReasonRBAC},
		{Check: CheckEvents, Status: SectionDenied, Reason: ReasonOff},
		{Check: CheckUsage, Status: SectionDenied},
		{Check: CheckNode, Status: SectionTimeout},
	}

	got := RuledOutOf(ruledOutIncident(), report, "")

	assert.Equal(t, []string{"kwatch cannot read pod logs here",
		"reading events is turned off in the configuration"},
		got.Unchecked)
}

func TestRuledOutSkipsANodeIncident(t *testing.T) {
	p := Incident{Root: inventory.CoreID(kube.KindNode, "", "n1"),
		Checked: []string{string(kube.KindZone)}}

	assert.True(t, RuledOutOf(p, nil, "quiet").Empty())
}
