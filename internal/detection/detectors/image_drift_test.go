package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// driftWorkload builds deployment "api" with one pod per image ID, all
// declaring the same image reference.
func driftWorkload(imageIDs ...string) (*inventory.Model, inventory.EntityID) {
	m := newTestModel()
	deploy := newID(kube.KindDeployment, "shop", "api")
	rs := newID(kube.KindReplicaSet, "shop", "api-1")
	put(m, deploy, t0, nil)
	put(m, rs, t0, nil)
	relateEntity(m, rs, inventory.OwnedBy, deploy)
	for i, imageID := range imageIDs {
		pod := newID(kube.KindPod, "shop", "api-1-"+string(rune('a'+i)))
		container := newID(kube.KindContainer, "shop", pod.Name+"/app")
		put(m, pod, t0, map[string]inventory.Value{
			kube.AttrPhase: inventory.Text("Running")})
		put(m, container, t0, map[string]inventory.Value{
			kube.AttrImage:   inventory.Text("registry/api:latest"),
			kube.AttrImageID: inventory.Text(imageID),
		})
		relateEntity(m, pod, inventory.OwnedBy, rs)
		relateEntity(m, container, inventory.PartOf, pod)
	}
	return m, deploy
}

func TestImageDriftReportsTwoBuildsUnderOneTag(t *testing.T) {
	m, deploy := driftWorkload("sha256:aaa", "sha256:bbb", "sha256:aaa")

	now := t0.Add(DefaultImageDriftGrace)
	got := ImageDrift{}.Detect(testDetectorContext(m, now), entityOf(m, deploy))

	require.Len(t, got, 1)
	assert.Equal(t, "ImageDigestDrift", got[0].Reason)
	assert.Equal(t, detection.Warning, got[0].Severity)
	assert.Contains(t, got[0].Summary, "2 different builds of image "+
		"registry/api:latest")
}

func TestImageDriftStaysQuietForOneBuild(t *testing.T) {
	m, deploy := driftWorkload("sha256:aaa", "sha256:aaa")

	assert.Empty(t, ImageDrift{}.Detect(testDetectorContext(m, t0),
		entityOf(m, deploy)))
}

func TestImageDriftWaitsOutTheGracePeriod(t *testing.T) {
	m, deploy := driftWorkload("sha256:aaa", "sha256:bbb")
	ctx := testDetectorContext(m, t0.Add(DefaultImageDriftGrace-time.Second))

	assert.Empty(t, ImageDrift{}.Detect(ctx, entityOf(m, deploy)))
}

func TestImageDriftWaitsForARolloutToFinish(t *testing.T) {
	m, deploy := driftWorkload("sha256:aaa", "sha256:bbb")
	// 1 of 2 replicas is updated: a rolling update is still in progress.
	put(m, deploy, t0, map[string]inventory.Value{
		kube.AttrReplicas:        inventory.Number(2),
		kube.AttrUpdatedReplicas: inventory.Number(1),
		kube.AttrAvailable:       inventory.Number(2),
	})
	ctx := testDetectorContext(m, t0.Add(time.Hour))

	assert.Empty(t, ImageDrift{}.Detect(ctx, entityOf(m, deploy)))
}

func TestImageDriftWaitsWhileTwoReplicaSetsHavePods(t *testing.T) {
	m, deploy := driftWorkload("sha256:aaa")
	rs2 := newID(kube.KindReplicaSet, "shop", "api-2")
	pod := newID(kube.KindPod, "shop", "api-2-a")
	container := newID(kube.KindContainer, "shop", "api-2-a/app")
	put(m, rs2, t0, nil)
	put(m, pod, t0, map[string]inventory.Value{
		kube.AttrPhase: inventory.Text("Running")})
	put(m, container, t0, map[string]inventory.Value{
		kube.AttrImage:   inventory.Text("registry/api:latest"),
		kube.AttrImageID: inventory.Text("sha256:bbb"),
	})
	relateEntity(m, rs2, inventory.OwnedBy, deploy)
	relateEntity(m, pod, inventory.OwnedBy, rs2)
	relateEntity(m, container, inventory.PartOf, pod)
	ctx := testDetectorContext(m, t0.Add(time.Hour))

	assert.Empty(t, ImageDrift{}.Detect(ctx, entityOf(m, deploy)))
}

func TestImageDriftReportsAfterTheRolloutSettled(t *testing.T) {
	m, deploy := driftWorkload("sha256:aaa", "sha256:bbb")
	put(m, deploy, t0, map[string]inventory.Value{
		kube.AttrReplicas:        inventory.Number(2),
		kube.AttrUpdatedReplicas: inventory.Number(2),
		kube.AttrAvailable:       inventory.Number(2),
	})
	ctx := testDetectorContext(m, t0.Add(time.Hour))

	assert.Len(t, ImageDrift{}.Detect(ctx, entityOf(m, deploy)), 1)
}

// Pods that fail because of the drifted build leave fewer replicas
// available than wanted. That must not hide the drift for ever.
func TestImageDriftReportsWhenReplicasAreUnavailableButNotRolling(
	t *testing.T,
) {
	m, deploy := driftWorkload("sha256:aaa", "sha256:bbb")
	put(m, deploy, t0, map[string]inventory.Value{
		kube.AttrReplicas:        inventory.Number(2),
		kube.AttrUpdatedReplicas: inventory.Number(2),
		kube.AttrAvailable:       inventory.Number(1),
	})
	ctx := testDetectorContext(m, t0.Add(time.Hour))

	assert.Len(t, ImageDrift{}.Detect(ctx, entityOf(m, deploy)), 1)
}
