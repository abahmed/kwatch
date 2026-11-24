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
			"-t registry.example/team/api:1.4-multiarch --push .",
		"kubectl set image deployment/api " +
			"app=registry.example/team/api:1.4-multiarch -n shop",
	}, stepCommands(steps)[:4])
	assert.False(t, steps[0].Mutating || steps[1].Mutating)
	assert.True(t, steps[2].Mutating && steps[3].Mutating)
	assert.Contains(t, steps[2].Text, "the current one is left as it is")
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

// A rebuild is never pushed over the tag the workload runs: the tag
// gains a suffix, a digest is dropped, and an untagged image gets one.
// Without the image there is nothing to tag and no build step.
func TestMultiArchStepsNeverOverwriteTheTag(t *testing.T) {
	for image, want := range map[string]string{
		"registry.example/team/api:1.4": "registry.example/team/api:" +
			"1.4-multiarch",
		"registry.example:5000/api": "registry.example:5000/api:multiarch",
		"api@sha256:abc":            "api:multiarch",
		"api:2@sha256:abc":          "api:2-multiarch",
	} {
		got, ok := multiArchTag(image)
		assert.True(t, ok, image)
		assert.Equal(t, want, got, image)
	}
	deploy := inventory.CoreID(kube.KindDeployment, "shop", "api")
	container := inventory.EntityID{Kind: kube.KindContainer,
		Namespace: "shop", Name: "api-1-x/app"}
	assert.Empty(t, multiArchSteps(deploy, container, "", "arm64", "",
		""))
	for _, s := range multiArchSteps(deploy, container,
		"registry.example/team/api:1.4", "arm64", "", "") {
		assert.NotContains(t, s.Command, "api:1.4 ")
		assert.NotContains(t, s.Command, "<")
	}
}

// A workload a GitOps controller or Helm deploys is pointed at the new
// tag where it is deployed from: a kubectl set image would be undone by
// the next sync. Only a workload deployed by other means gets it.
func TestMultiArchTagGoesToGitForAManagedWorkload(t *testing.T) {
	deploy := inventory.CoreID(kube.KindDeployment, "shop", "api")
	container := inventory.EntityID{Kind: kube.KindContainer,
		Namespace: "shop", Name: "api-1-x/app"}
	image := "registry.example/team/api:1.4"
	for app, want := range map[string]string{
		"argocd/shop":   "in Git (argocd/shop)",
		"flux/shop":     "in Git (flux/shop)",
		"helm/shop/api": "in the values of Helm release api",
	} {
		steps := multiArchSteps(deploy, container, image, "arm64", "", app)
		last := steps[len(steps)-1]
		assert.Contains(t, last.Text, "container app to "+
			"registry.example/team/api:1.4-multiarch "+want, app)
		assert.Empty(t, last.Command, app)
		assert.False(t, last.Mutating, app)
	}
	steps := multiArchSteps(deploy, container, image, "arm64", "", "")
	assert.Contains(t, steps[len(steps)-1].Command, "kubectl set image")
}

// A workload a GitOps controller or Helm deploys is pinned where it is
// deployed from: a kubectl patch of its nodeSelector would be undone by
// the next sync or upgrade. Only a workload deployed by other means is
// patched.
func TestArchPinGoesToGitForAManagedWorkload(t *testing.T) {
	deploy := inventory.CoreID(kube.KindDeployment, "shop", "api")
	for app, want := range map[string]string{
		"argocd/shop":   "in Git (argocd/shop)",
		"flux/shop":     "in Git (flux/shop)",
		"helm/shop/api": "in the values of Helm release api",
	} {
		pin, ok := archPin(deploy, "amd64", app)
		require.True(t, ok, app)
		assert.Contains(t, pin.Text,
			"nodeSelector kubernetes.io/arch=amd64 "+want, app)
		assert.Empty(t, pin.Command, app)
		assert.False(t, pin.Mutating, app)
	}
	pin, ok := archPin(deploy, "amd64", "")
	require.True(t, ok)
	assert.Contains(t, pin.Command, "kubectl patch deployment api")
	assert.True(t, pin.Mutating)
}
