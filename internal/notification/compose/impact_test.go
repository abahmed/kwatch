package compose

import (
	"testing"

	"github.com/stretchr/testify/assert"

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
		"batch in shop is affected as well."}, texts)
}

func TestImpactKeepsAWorkloadNameAsWritten(t *testing.T) {
	f := impactFacts(workload("storage"), workload("support"))

	got := arrange(impactSentences(f))

	assert.Equal(t, "storage and support in shop are affected as well.",
		got[0].text)
}
