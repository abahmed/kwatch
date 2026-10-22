package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/feature"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// proPodReasons are the pod reasons moved out of Community.
var proPodReasons = []string{
	reasons.PodPreempted, reasons.PodPreemptedRepeatedly,
	reasons.PodAdmissionRejected, reasons.PodKilledAtGrace,
	reasons.SchedulingGated,
}

var movedAt = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

func editionPair(
	det detection.Detector, governs func(string) feature.ID,
	withheld *feature.Withheld,
) (community, pro restricted) {
	community = restricted{Detector: det, governs: governs,
		entitled: feature.CommunityEntitlements(), withheld: withheld}
	pro = restricted{Detector: det, governs: governs,
		entitled: feature.ProEntitlements(), withheld: withheld}
	return community, pro
}

func TestCommunityPodDetectionDropsRareReasons(t *testing.T) {
	found := append([]string{reasons.PodFailed}, proPodReasons...)
	pod := fixed{Detector: NewPod(PodThresholds{}),
		found: findingsOf(found...)}
	withheld := feature.NewWithheld()
	community, pro := editionPair(pod, podGoverns, withheld)
	ctx := detection.Context{Now: movedAt}
	e := inventory.Entity{ID: inventory.CoreID(kube.KindPod, "a", "p")}

	assert.Equal(t, []string{reasons.PodFailed},
		findingReasons(community.Detect(ctx, e)))
	assert.Equal(t, feature.WithheldCounts{0, 0, 4, 1},
		withheld.Counts(movedAt), "community counts what it drops")
	assert.Equal(t, found, findingReasons(pro.Detect(ctx, e)))
	assert.Equal(t, feature.WithheldCounts{0, 0, 4, 1},
		withheld.Counts(movedAt), "pro withholds nothing")
}

func TestCommunityContainerDropsOnlyReadinessProbes(t *testing.T) {
	container := fixed{Detector: Container{}, found: findingsOf(
		reasons.ReadinessProbeFailed, reasons.LivenessProbeFailed,
		reasons.StartupProbeFailed, reasons.CrashLoopBackOff)}
	withheld := feature.NewWithheld()
	community, pro := editionPair(container, containerGoverns, withheld)
	ctx := detection.Context{Now: movedAt}

	assert.Equal(t, []string{reasons.LivenessProbeFailed,
		reasons.StartupProbeFailed, reasons.CrashLoopBackOff},
		findingReasons(community.Detect(ctx, inventory.Entity{})))
	assert.Equal(t, 1, withheld.Counts(movedAt)[feature.WithheldReadiness])
	assert.Len(t, pro.Detect(ctx, inventory.Entity{}), 4)
}

func TestEntitledWrapsContainerAndPodInCommunity(t *testing.T) {
	for _, d := range Entitled(feature.CommunityEntitlements(), nil) {
		if d.Name() == "container" || d.Name() == "pod" {
			_, ok := d.(restricted)
			assert.True(t, ok, d.Name())
		}
	}
	for _, d := range Entitled(feature.ProEntitlements(), nil) {
		_, ok := d.(restricted)
		assert.False(t, ok, d.Name())
	}
}

func TestCommunityGatedPodRaisesNothing(t *testing.T) {
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	since := now.Add(-20 * time.Minute)
	model := newTestModel()
	id := inventory.EntityID{Kind: kube.KindPod, Namespace: "default",
		Name: "gated"}
	condKey := kube.ConditionKey("PodScheduled")
	model.Apply(inventory.Observation{
		Kind: inventory.Observed, Source: "test", At: since, Entity: id,
		Attributes: map[string]inventory.Value{
			kube.AttrPhase: inventory.Text("Pending"),
			condKey:        inventory.Text("False"),
			condKey + kube.AttrConditionReason: inventory.Text(
				reasons.SchedulingGated),
			condKey + kube.AttrConditionSince: inventory.Time(since),
		},
	})
	entity, _ := model.Entity(id)
	ctx := testDetectorContext(model, now)
	community, pro := editionPair(NewPod(PodThresholds{}), podGoverns,
		nil)

	assert.Empty(t, community.Detect(ctx, entity),
		"a gated pod is not unschedulable in Community")
	got := pro.Detect(ctx, entity)
	require.Len(t, got, 1)
	assert.Equal(t, reasons.SchedulingGated, got[0].Reason)
}
