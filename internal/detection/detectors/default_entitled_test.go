package detectors

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/feature"
	"github.com/abahmed/kwatch/internal/inventory"
)

func names(list []detection.Detector) []string {
	var out []string
	for _, d := range list {
		out = append(out, d.Name())
	}
	return out
}

func TestEveryEntryHasAFeature(t *testing.T) {
	entries := Entries()
	require.Len(t, entries, len(Default()))
	for _, entry := range entries {
		assert.NotEmpty(t, entry.Feature, entry.Detector.Name())
		if entry.Governs != nil {
			assert.NotEmpty(t, entry.Also, entry.Detector.Name())
		}
	}
}

func TestProRunsEveryDetector(t *testing.T) {
	assert.Equal(t, names(Default()),
		names(Entitled(feature.ProEntitlements(), nil)))
}

func TestNilEntitlementsRunCommunityDetectors(t *testing.T) {
	assert.Equal(t, names(Entitled(feature.CommunityEntitlements(), nil)),
		names(Entitled(nil, nil)))
	assert.NotEqual(t, names(Default()), names(Entitled(nil, nil)))
}

func TestCommunityRunsOnlyPodAndNodeDetectors(t *testing.T) {
	got := names(Entitled(feature.CommunityEntitlements(), nil))
	assert.Equal(t, []string{"container", "init-wait", "pod", "node"}, got)
}

type fixed struct {
	detection.Detector
	found []detection.Finding
}

func (f fixed) Detect(detection.Context, inventory.Entity) []detection.Finding {
	return f.found
}

func findingsOf(names ...string) []detection.Finding {
	var out []detection.Finding
	for _, name := range names {
		out = append(out, detection.Finding{Reason: name})
	}
	return out
}

func TestCommunityNodeDetectionIsNotReadyOnly(t *testing.T) {
	node := fixed{Detector: NewNode(0), found: findingsOf(
		reasons.NodeNotReady, reasons.NodeMemoryPressure,
		reasons.NodeDraining, reasons.NodeFilesystemHigh,
		reasons.NodeStuckTerminating)}
	community := restricted{Detector: node,
		entitled: feature.CommunityEntitlements(), governs: nodeGoverns}
	pro := restricted{Detector: node,
		entitled: feature.ProEntitlements(), governs: nodeGoverns}

	assert.Equal(t, []string{reasons.NodeNotReady},
		findingReasons(community.Detect(detection.Context{}, inventory.Entity{})))
	assert.Len(t, pro.Detect(detection.Context{}, inventory.Entity{}), 5)
}

func TestCommunityPodDetectionDropsReadinessOnly(t *testing.T) {
	pod := fixed{Detector: NewPod(PodThresholds{}), found: findingsOf(
		reasons.PodPending, reasons.ContainersNotReady,
		reasons.PodFailed)}
	community := restricted{Detector: pod,
		entitled: feature.CommunityEntitlements(), governs: podGoverns}

	assert.Equal(t, []string{reasons.PodPending, reasons.PodFailed},
		findingReasons(community.Detect(detection.Context{}, inventory.Entity{})))
}
