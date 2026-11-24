package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// hpaTargetRig is an autoscaler whose target Deployment exists only when
// present is true.
func hpaTargetRig(present bool) (*inventory.Model, inventory.EntityID) {
	model := newTestModel()
	hpa := newID(kube.KindHPA, "istio-system", "istiod")
	target := newID(kube.KindDeployment, "istio-system", "istiod")
	put(model, hpa, t0, map[string]inventory.Value{
		kube.AttrCurrentReplicas: inventory.Number(0)})
	setCondition(model, hpa, "AbleToScale", "False", "FailedGetScale",
		`deployments.apps "istiod" not found`, t0)
	if present {
		put(model, target, t0, nil)
	}
	relateEntity(model, hpa, inventory.Scales, target)
	return model, hpa
}

func detectHPAAt(
	model *inventory.Model, id inventory.EntityID, after time.Duration,
) []detection.Finding {
	ctx := testDetectorContext(model, t0.Add(after))
	return HPA{}.Detect(ctx, entityOf(model, id))
}

func TestHPAMissingTargetIsDigestNews(t *testing.T) {
	model, hpa := hpaTargetRig(false)

	got := detectHPAAt(model, hpa, 2*DefaultConditionGrace)

	require.Len(t, got, 1, "FailedGetScale is not reported twice")
	assert.Equal(t, reasons.HPATargetMissing, got[0].Reason)
	assert.Equal(t, detection.Info, got[0].Severity)
	assert.Equal(t, "HPA istio-system/istiod targets Deployment istiod, "+
		"which does not exist.", got[0].Summary)
	assert.Equal(t, `deployments.apps "istiod" not found`,
		got[0].Evidence[0].Value)
}

func TestHPAMissingTargetWaitsForTheGrace(t *testing.T) {
	model, hpa := hpaTargetRig(false)

	got := detectHPAAt(model, hpa, 0)

	assert.NotContains(t, findingReasons(got), reasons.HPATargetMissing)
}

func TestHPAWithExistingTargetReportsNoMissingTarget(t *testing.T) {
	model, hpa := hpaTargetRig(true)

	got := detectHPAAt(model, hpa, 2*DefaultConditionGrace)

	assert.NotContains(t, findingReasons(got), reasons.HPATargetMissing)
	assert.Contains(t, findingReasons(got), reasons.FailedGetScale)
}

func TestHPAMissingTargetOfAnUnwatchedKindIsNotConcluded(t *testing.T) {
	model := newTestModel()
	hpa := newID(kube.KindHPA, "apps", "rollout-hpa")
	put(model, hpa, t0, nil)
	relateEntity(model, hpa, inventory.Scales,
		newID("rollout", "apps", "web"))

	got := detectHPAAt(model, hpa, time.Hour)

	assert.Empty(t, got)
}

// A scaleTargetRef whose apiVersion the cluster does not serve names a
// target that exists under another version: it is not missing, and the
// finding says why the autoscaler cannot read it.
func TestHPATargetInUnservedVersionSaysSo(t *testing.T) {
	model, hpa := hpaTargetRig(true)
	setCondition(model, hpa, "AbleToScale", "False", "FailedGetScale",
		`the HPA controller was unable to get the target's current `+
			`scale: no matches for kind "Deployment" in version "apps/v2"`,
		t0)

	got := detectHPAAt(model, hpa, 2*DefaultConditionGrace)

	require.Len(t, got, 1)
	assert.Equal(t, reasons.FailedGetScale, got[0].Reason)
	assert.Equal(t, "Autoscaler cannot read its scale target: the API "+
		"version its scaleTargetRef names is not served", got[0].Summary)
}
