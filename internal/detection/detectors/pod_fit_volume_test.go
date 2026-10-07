package detectors

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

const zoneTerms = `[[{"k":"topology.kubernetes.io/zone","o":"In",` +
	`"v":["eu-west-1b"]}]]`

// mountClaim gives pod a claim bound to a volume restricted to terms.
func mountClaim(m *inventory.Model, pod inventory.EntityID, terms string) {
	claim := inventory.CoreID(kube.KindPVC, "d", "data-0")
	volume := inventory.CoreID(kube.KindPV, "", "pv-1")
	observeEntity(m, claim, podNodeNow, map[string]inventory.Value{
		kube.AttrPhase: inventory.Text("Bound")})
	observeEntity(m, volume, podNodeNow, map[string]inventory.Value{
		kube.AttrNodeTerms: inventory.Text(terms)})
	relateEntity(m, claim, inventory.References, volume)
	relateEntity(m, pod, inventory.Mounts, claim)
}

func TestFitVolumeZoneHasNoNodes(t *testing.T) {
	m := newTestModel()
	fitNodeIn(m, "n1", "general", "eu-west-1a", 4000, nil)
	fitNodeIn(m, "n2", "general", "eu-west-1c", 4000, nil)
	pod := fitPending(m, "p", 1000, kube.SchedulingSpec{})
	mountClaim(m, pod, zoneTerms)
	assert.Equal(t, []string{
		"claim data-0 (volume pv-1) can only attach where " +
			"topology.kubernetes.io/zone=eu-west-1b, and no node is there"},
		fitLines(fitOf(t, m, pod), detection.EvidenceFit))
}

func TestFitVolumeZoneWithNodeThatIsFull(t *testing.T) {
	m := newTestModel()
	fitNodeIn(m, "n1", "general", "eu-west-1a", 4000, nil)
	fitNodeIn(m, "n2", "general", "eu-west-1b", 1000, nil)
	capPod(m, "x", "n2", "Running", 900, 0)
	pod := fitPending(m, "p", 1000, kube.SchedulingSpec{})
	mountClaim(m, pod, zoneTerms)
	got := fitLines(fitOf(t, m, pod), detection.EvidenceFit)
	assert.Equal(t, []string{
		"claim data-0 (volume pv-1) can only attach where " +
			"topology.kubernetes.io/zone=eu-west-1b; the only node " +
			"there, n2, has only 100m CPU free of the 1 CPU it needs"},
		got)
}

func TestFitUnboundClaimUsesStorageClassTopology(t *testing.T) {
	m := newTestModel()
	fitNodeIn(m, "n1", "general", "eu-west-1a", 4000, nil)
	pod := fitPending(m, "p", 1000, kube.SchedulingSpec{})
	claim := inventory.CoreID(kube.KindPVC, "d", "data-0")
	class := inventory.CoreID(kube.KindStorageClass, "", "ebs")
	observeEntity(m, claim, podNodeNow, map[string]inventory.Value{
		kube.AttrPhase: inventory.Text("Pending")})
	observeEntity(m, class, podNodeNow, map[string]inventory.Value{
		kube.AttrNodeTerms: inventory.Text(zoneTerms)})
	relateEntity(m, claim, inventory.References, class)
	relateEntity(m, pod, inventory.Mounts, claim)
	assert.Len(t, fitLines(fitOf(t, m, pod), detection.EvidenceFit), 1)
}
