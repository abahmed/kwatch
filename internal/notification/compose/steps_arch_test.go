package compose

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/notification"
)

func stepCommands(steps []notification.Step) []string {
	var out []string
	for _, s := range steps {
		out = append(out, s.Command)
	}
	return out
}

// An image not built for the node's CPU leads with how to check that
// (the image's manifest, each node's CPU), then the fixes, labelled as
// changes; the plain crash steps follow. Without healthy replicas
// kwatch does not know a CPU the image runs on, so it pins nothing.
func TestArchStepsLeadWithTheCPUCheck(t *testing.T) {
	p := archIncident("")

	steps := nextSteps(p, sortedMembers(p))

	require.GreaterOrEqual(t, len(steps), 3)
	assert.Equal(t, "See which CPU architectures the image is built "+
		"for (it fails on arm64): docker manifest inspect "+
		"registry.example/team/api:1.4, or crane manifest "+
		"registry.example/team/api:1.4", steps[0].Text)
	assert.Equal(t, []string{"",
		"kubectl get nodes -L kubernetes.io/arch",
		"docker buildx build --platform linux/amd64,linux/arm64 " +
			"-t registry.example/team/api:1.4 --push .",
	}, stepCommands(steps)[:3])
	assert.False(t, steps[0].Mutating || steps[1].Mutating)
	assert.True(t, steps[2].Mutating)
	assert.Contains(t, steps[2].Text, "replaces the image")
	for _, s := range steps {
		assert.NotContains(t, s.Command, "nodeSelector")
	}
}

// Replicas that run fine on amd64 nodes prove a CPU the image runs on:
// the workload may be pinned there, as a labelled change.
func TestArchStepsPinToTheCPUItsReplicasRunOn(t *testing.T) {
	p := archIncident("2 pods on amd64 nodes")

	steps := nextSteps(p, sortedMembers(p))

	require.GreaterOrEqual(t, len(steps), 4)
	pin := steps[2]
	assert.True(t, pin.Mutating)
	assert.Equal(t, "kubectl patch deployment api -n shop -p "+
		`'{"spec":{"template":{"spec":{"nodeSelector":`+
		`{"kubernetes.io/arch":"amd64"}}}}}'`, pin.Command)
	assert.Contains(t, pin.Text, "changes the cluster")

	bare := archIncident("2 pods on amd64 nodes")
	bare.Root = inventory.CoreID(kube.KindPod, "shop", "api-1-x")
	for _, s := range nextSteps(bare, sortedMembers(bare)) {
		assert.NotContains(t, s.Command, "kubectl patch",
			"a bare pod has no template to pin")
	}
}

func TestHealthyArchReadsTheLargestCount(t *testing.T) {
	assert.Equal(t, "amd64",
		healthyArch("2 pods on amd64 nodes and 1 pod on s390x nodes"))
	assert.Equal(t, "", healthyArch(""))
	assert.Equal(t, "", healthyArch("pods on many nodes"))
}
