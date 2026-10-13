package detection

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func containerWith(attrs map[string]inventory.Value) inventory.Entity {
	e := inventory.Entity{ID: inventory.CoreID(kube.KindContainer, "shop",
		"api-1/app"), Attributes: map[string]inventory.Attribute{}}
	for name, value := range attrs {
		e.Attributes[name] = inventory.Attribute{Value: value}
	}
	return e
}

func TestAMemoryKillNamesTheLimitItReached(t *testing.T) {
	e := containerWith(map[string]inventory.Value{
		kube.AttrMemoryLimit: inventory.Number(256 << 20)})

	got := withPlace(e, Finding{Reason: reasons.OOMKilled})

	assert.Equal(t, &Place{Container: "app",
		Path: "resources.limits.memory", Value: "256Mi"}, got.Place)
}

func TestAKillByTheNodeNamesNoLimit(t *testing.T) {
	e := containerWith(map[string]inventory.Value{
		kube.AttrMemoryLimit: inventory.Number(256 << 20)})

	got := withPlace(e, Finding{Reason: reasons.OOMKilled,
		Evidence: []Evidence{{Label: EvidenceKilledByNode, Value: "n1"}}})

	assert.Nil(t, got.Place)
}

func TestAThrottledContainerNamesItsCPULimit(t *testing.T) {
	e := containerWith(map[string]inventory.Value{
		kube.AttrCPULimit: inventory.Number(200)})

	got := withPlace(e, Finding{Reason: reasons.LivenessKilled,
		Evidence: []Evidence{{Label: EvidenceCPUThrottled, Value: "72%"}}})

	assert.Equal(t, &Place{Container: "app", Path: "resources.limits.cpu",
		Value: "200m"}, got.Place)
}

func TestNoPlaceWithoutALimitOrForOtherEntitiesAndAdvisories(t *testing.T) {
	assert.Nil(t, withPlace(containerWith(nil),
		Finding{Reason: reasons.OOMKilled}).Place)

	pod := inventory.Entity{ID: inventory.CoreID(kube.KindPod, "shop", "p"),
		Attributes: map[string]inventory.Attribute{
			kube.AttrMemoryLimit: {Value: inventory.Number(1 << 30)}}}
	assert.Nil(t, withPlace(pod, Finding{Reason: reasons.OOMKilled}).Place)

	limited := containerWith(map[string]inventory.Value{
		kube.AttrMemoryLimit: inventory.Number(1 << 30)})
	assert.Nil(t, withPlace(limited, Finding{Reason: reasons.OOMKilled,
		Advisory: true}).Place)
}

func TestMemoryAndCPUText(t *testing.T) {
	assert.Equal(t, "512Mi", MemoryText(512<<20))
	assert.Equal(t, "1.5Gi", MemoryText(1536<<20))
	assert.Equal(t, "2Gi", MemoryText(2<<30))
	assert.Equal(t, "250m", CPUText(250))
	assert.Equal(t, "2", CPUText(2000))
	assert.Equal(t, "1500m", CPUText(1500))
}

func TestAPlaceNeedsAPath(t *testing.T) {
	assert.False(t, Place{Container: "app"}.Valid())
	assert.True(t, Place{Path: "image"}.Valid())
}
