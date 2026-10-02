package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func scale(before, after string) inventory.Change {
	return inventory.Change{Fields: []inventory.FieldChange{{
		Path: "replicas", Before: before, After: after,
	}}}
}

// An autoscaler adding replicas after a bad rollout neither changes the
// blamed change, nor the digest, nor is it named as the fix.
func TestManagerAutoscalerAfterBadRolloutIsNotNews(t *testing.T) {
	r := newRig(t, Config{})
	web := podSig("web")
	web.Health = detection.Failing
	deploy := entity(kube.KindDeployment, "web")
	rollout := image("web:v1", "web:v2")
	rollout.Entity, rollout.At = deploy, at(-30*time.Second)
	c := r.cause(web.Entity, deploy, "rollout of web:v2 crashes")
	c.Changes = []inventory.Change{rollout}
	r.rule.causes[web.Entity] = c
	announced(t, r, web)
	before := r.only()
	require.NotNil(t, before.Cause.Change)

	autoscale := scale("2", "6")
	autoscale.Entity, autoscale.At = deploy, at(2*time.Minute)
	c.Changes = []inventory.Change{rollout, autoscale}
	r.rule.causes[web.Entity] = c
	r.apply(at(2*time.Minute), detection.Changed, web)

	wantNone(t, r.tick(at(3*time.Minute)))
	after := r.of(deploy)
	assert.Equal(t, before.Digest, after.Digest)
	assert.Equal(t, rollout.At, after.Cause.Change.At)
}

func TestResolveDoesNotNameAnAutoscalerAsTheFix(t *testing.T) {
	r := newRig(t, Config{})
	web := podSig("web")
	r.observe(web.Entity)
	r.change(web.Entity, -30*time.Second, image("web:v1", "web:v2"))
	r.change(web.Entity, 2*time.Minute, scale("2", "6"))
	failOnce(r, web, 0, 4*time.Minute)

	got := r.of(web.Entity)
	assert.Nil(t, got.FixedBy, "a scale is not a fix")
	assert.Equal(t, FixNone, got.Fix)
}

// A cause that names the incident's own root, and nothing it does not
// already report, is not news: the first victim joining does not
// re-send the announcement.
func TestManagerCauseNamingTheAnnouncedRootIsNotNews(t *testing.T) {
	r := newRig(t, Config{})
	hook := sig(entity(kube.KindValidatingHook, "policy"),
		"WebhookNoEndpoints", detection.Critical)
	hook.Health = detection.Failing
	announced(t, r, hook)

	victim := podSig("orders")
	r.cause(victim.Entity, hook.Entity, "webhook rejects pod creation")
	r.raise(at(2*time.Minute), victim)
	got := r.of(hook.Entity)
	require.NotNil(t, got.Cause, "the victim brings the cause")
	wantNone(t, r.tick(at(3*time.Minute)))
}
