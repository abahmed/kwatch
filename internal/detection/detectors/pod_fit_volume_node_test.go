package detectors

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

const hostTerms = `[[{"k":"kubernetes.io/hostname","o":"In",` +
	`"v":["n9"]}]]`

func TestFitLocalVolumeNodeIsGone(t *testing.T) {
	m := newTestModel()
	fitNodeIn(m, "n1", "general", "eu-west-1a", 4000, nil)
	pod := fitPending(m, "p", 1000, kube.SchedulingSpec{})
	mountClaim(m, pod, hostTerms)
	assert.Equal(t, []string{
		"claim data-0 (volume pv-1) lives on node n9, which is gone"},
		fitLines(fitOf(t, m, pod), detection.EvidenceFit))
}

func TestFitLocalVolumeNodeNotReady(t *testing.T) {
	m := newTestModel()
	fitNodeIn(m, "n1", "general", "eu-west-1a", 4000, nil)
	fitNodeIn(m, "n9", "general", "eu-west-1a", 4000,
		map[string]inventory.Value{kube.AttrReady: inventory.Bool(false)})
	pod := fitPending(m, "p", 1000, kube.SchedulingSpec{})
	mountClaim(m, pod, hostTerms)
	assert.Equal(t, []string{
		"claim data-0 (volume pv-1) lives on node n9, which is NotReady"},
		fitLines(fitOf(t, m, pod), detection.EvidenceFit))
}

func TestFitVolumeZoneNamesClosestOfSeveralNodes(t *testing.T) {
	m := newTestModel()
	fitNodeIn(m, "n1", "general", "eu-west-1b", 1000, nil)
	fitNodeIn(m, "n2", "general", "eu-west-1b", 1000, nil)
	fitNodeIn(m, "n3", "general", "eu-west-1a", 4000, nil)
	capPod(m, "x", "n1", "Running", 900, 0)
	capPod(m, "y", "n2", "Running", 950, 0)
	pod := fitPending(m, "p", 1000, kube.SchedulingSpec{})
	mountClaim(m, pod, zoneTerms)
	assert.Equal(t, []string{
		"claim data-0 (volume pv-1) can only attach where " +
			"topology.kubernetes.io/zone=eu-west-1b; the closest of " +
			"2 nodes there, n1, has only 100m CPU free of the 1 CPU " +
			"it needs"},
		fitLines(fitOf(t, m, pod), detection.EvidenceFit))
}
