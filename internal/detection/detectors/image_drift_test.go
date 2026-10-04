package detectors

import (
	"testing"

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

	got := ImageDrift{}.Detect(testDetectorContext(m, t0), entityOf(m, deploy))

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
