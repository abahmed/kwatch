package compose

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/feature"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func impactFacts(impact ...inventory.EntityID) caseFacts {
	node := inventory.CoreID(kube.KindNode, "", "n3")
	return caseFacts{p: incident.Incident{Root: node, Impact: impact}}
}

func service(name string) inventory.EntityID {
	return inventory.CoreID(kube.KindService, "shop", name)
}

func workload(name string) inventory.EntityID {
	return inventory.CoreID(kube.KindDeployment, "shop", name)
}

// A workload whose own Service is listed as unable to serve is
// affected by definition: its name is not said twice.
func TestImpactDoesNotListAWorkloadBesideItsOwnService(t *testing.T) {
	f := impactFacts(service("fleet"), service("storage"),
		workload("fleet"), workload("storage"), workload("batch"))

	got := impactSentences(f)

	var texts []string
	for _, s := range got {
		texts = append(texts, s.text)
	}
	assert.Equal(t, []string{
		"Services fleet and storage in shop can't serve traffic.",
		"Affected: batch in shop."}, texts)
}

func TestImpactKeepsAWorkloadNameAsWritten(t *testing.T) {
	f := impactFacts(workload("storage"), workload("support"))

	got := arrange(impactSentences(f))

	assert.Equal(t, "Affected: storage and support in shop.",
		got[0].text)
}

// noImpact is an edition that has everything e has except Impact.
type noImpact struct{ feature.Entitlements }

func (n noImpact) Has(id feature.ID) bool {
	return id != feature.Impact && n.Entitlements.Has(id)
}

// What else a failure affected is said only with the Impact
// capability: Pro has it, Community does not.
func TestWriterSaysWhatWasAffectedOnlyWithImpact(t *testing.T) {
	d := runbookDecision(incident.Resolve, "OOMKilled")
	d.Incident.Opened = d.Incident.Resolved.Add(-10 * time.Minute)
	d.Incident.Journal.Widest = incident.Radius{Pods: 3}
	const affected = "Three pods were affected."
	for name, tc := range map[string]struct {
		e    feature.Entitlements
		says bool
	}{
		"pro":            {feature.ProEntitlements(), true},
		"pro w/o impact": {noImpact{feature.ProEntitlements()}, false},
		"community":      {feature.CommunityEntitlements(), false},
	} {
		w := Writer{}.ForEdition(tc.e, true)
		note := w.Write(d, d.Incident.Resolved).Note
		assert.Equal(t, tc.says, strings.Contains(note, affected),
			"%s: %s", name, note)
	}
	assert.Equal(t, Writer{}.Write(d, d.Incident.Resolved),
		Writer{}.ForEdition(feature.ProEntitlements(), true).
			Write(d, d.Incident.Resolved), "Pro is unchanged")
}
