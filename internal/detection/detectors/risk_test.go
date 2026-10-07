package detectors

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// riskWorkload builds deployment "api" with replicas pods on the given
// nodes, each with one main container carrying attrs.
func riskWorkload(
	replicas float64, nodes []string, attrs map[string]inventory.Value,
) (*inventory.Model, inventory.EntityID) {
	m := newTestModel()
	deploy := newID(kube.KindDeployment, "shop", "api")
	rs := newID(kube.KindReplicaSet, "shop", "api-1")
	put(m, deploy, t0, map[string]inventory.Value{
		kube.AttrReplicas: inventory.Number(replicas)})
	put(m, rs, t0, nil)
	relateEntity(m, rs, inventory.OwnedBy, deploy)
	for i, node := range nodes {
		pod := newID(kube.KindPod, "shop", "api-1-"+string(rune('a'+i)))
		container := newID(kube.KindContainer, "shop", pod.Name+"/app")
		put(m, pod, t0, map[string]inventory.Value{
			kube.AttrPhase: inventory.Text("Running")})
		put(m, container, t0, attrs)
		relateEntity(m, pod, inventory.OwnedBy, rs)
		relateEntity(m, pod, inventory.RunsOn, newID(kube.KindNode, "", node))
		relateEntity(m, container, inventory.PartOf, pod)
	}
	return m, deploy
}

func riskReasons(findings []detection.Finding) []string {
	var out []string
	for _, f := range findings {
		out = append(out, f.Reason)
	}
	return out
}

func TestRiskReportsEveryRiskOnce(t *testing.T) {
	m, deploy := riskWorkload(1, []string{"n1"},
		map[string]inventory.Value{
			kube.AttrImage: inventory.Text("registry/api:latest"),
		})

	got := Risk{}.Detect(testDetectorContext(m, t0), entityOf(m, deploy))

	assert.ElementsMatch(t, []string{
		reasons.RiskNoReadinessProbe, reasons.RiskNoMemoryLimit,
		reasons.RiskMutableImageTag, reasons.RiskSingleReplica,
	}, riskReasons(got))
	for _, f := range got {
		assert.True(t, f.Advisory, f.Reason)
		assert.Equal(t, detection.Info, f.Severity, f.Reason)
	}
}

func TestRiskSingleReplicaAndWellConfiguredWorkload(t *testing.T) {
	m, deploy := riskWorkload(1, []string{"n1"},
		map[string]inventory.Value{
			kube.AttrImage:       inventory.Text("registry/api:2.4"),
			kube.AttrProbes:      inventory.Text("readiness,liveness"),
			kube.AttrMemoryLimit: inventory.Number(512 << 20),
		})

	got := Risk{}.Detect(testDetectorContext(m, t0), entityOf(m, deploy))

	assert.Equal(t, []string{reasons.RiskSingleReplica}, riskReasons(got))
}

func TestRiskStaysQuietWithoutPods(t *testing.T) {
	m := newTestModel()
	deploy := newID(kube.KindDeployment, "shop", "api")
	put(m, deploy, t0, map[string]inventory.Value{
		kube.AttrReplicas: inventory.Number(1)})

	assert.Empty(t, Risk{}.Detect(testDetectorContext(m, t0),
		entityOf(m, deploy)))
}

func TestMutableTag(t *testing.T) {
	for image, want := range map[string]bool{
		"registry/api":                      true,
		"registry/api:latest":               true,
		"registry/api:2.4":                  false,
		"registry:5000/api":                 true,
		"registry:5000/api:2.4":             false,
		"registry/api@sha256:0123456789ab":  false,
		"registry/api:latest@sha256:012345": false,
	} {
		assert.Equal(t, want, mutableTag(image), image)
	}
}

// Three replicas of one container are one container to fix, not three.
func TestRiskCountsUnlimitedContainersPerTemplate(t *testing.T) {
	m, deploy := riskWorkload(3, []string{"n1", "n2", "n3"},
		map[string]inventory.Value{
			kube.AttrImage:  inventory.Text("registry/api:2.4"),
			kube.AttrProbes: inventory.Text("readiness"),
		})

	got := Risk{}.Detect(testDetectorContext(m, t0), entityOf(m, deploy))

	require.Len(t, got, 1)
	assert.Contains(t, got[0].Summary, "1 of its containers")
}
