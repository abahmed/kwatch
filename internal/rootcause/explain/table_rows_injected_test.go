package explain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// injectedPull builds a workload whose template names one image, with a
// pod that fails to pull pulled; a mutating webhook configuration
// changed at minute 0 and the pull fails from minute 2.
func injectedPull(f *fixture, pulled string) (inventory.EntityID,
	inventory.EntityID) {
	pod := f.workload("shop", "api", 1)[0]
	f.apply(inventory.Observation{Kind: inventory.Observed,
		Entity: inventory.CoreID(kube.KindDeployment, "shop", "api"),
		Attributes: map[string]inventory.Value{
			kube.AttrTemplateImages: inventory.Text("r.example/app:1")}})
	f.relate(pod, inventory.Pulls,
		inventory.CoreID(kube.KindImage, "", "r.example/app:1"),
		inventory.CoreID(kube.KindImage, "", pulled))
	f.fail(containerOf(pod), detection.ModeImagePull, failingH, 2,
		"not found")
	hook := inventory.CoreID(kube.KindMutatingWebhook, "", "injector")
	f.add(hook)
	f.change(hook, 0, "webhooks[0].clientConfig")
	return containerOf(pod), hook
}

// An image the template does not name, failing right after a mutating
// webhook configuration changed, is the webhook's.
func TestUntemplatedImageAfterWebhookChangeBlamesTheWebhook(t *testing.T) {
	f := newFixture(t)
	effect, hook := injectedPull(f, "r.example/proxy:9")

	c, ok := rootOf(f, effect)

	require.True(t, ok)
	assert.Equal(t, hook, c.Root)
	assert.Equal(t, RuleWebhookInjectsImage, c.Row)
}

// An image the template names is not injected: the registry is blamed.
func TestTemplatedImageFailureIsNotTheWebhooks(t *testing.T) {
	f := newFixture(t)
	effect, hook := injectedPull(f, "r.example/app:1")

	c, ok := rootOf(f, effect)

	assert.False(t, ok && c.Root == hook)
}

// injectedRowCases: a pod pulls an image its template does not name,
// right after a mutating webhook configuration changed.
var injectedRowCases = []rowCase{
	{row: RuleWebhookInjectsImage, want: "mutatingwebhookconfiguration//injector",
		build: func(f *fixture) inventory.EntityID {
			effect, _ := injectedPull(f, "r.example/proxy:9")
			return effect
		}},
}
