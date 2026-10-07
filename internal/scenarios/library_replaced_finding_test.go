package scenarios

import (
	"time"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// replacedFindingScenarios are announced incidents whose findings are
// later replaced by quieter ones.
func replacedFindingScenarios() []scenario {
	return []scenario{scaleFailureReplaced()}
}

// scaleFailureReplaced: an autoscaler names a scale target kind kwatch
// cannot look up, so the controller's own FailedGetScale is all there is
// and the incident notifies. Someone then fixes the kind to a Deployment
// that does not exist, and kwatch can say so: an informational finding.
// The incident falls to the digest and its thread hears the cause once.
func scaleFailureReplaced() scenario {
	return scenario{
		expect: expectation{
			Name: "scale-failure-replaced",
			Description: "An HPA first fails with the controller's " +
				"FailedGetScale, then names a Deployment that is gone.",
			Root: "horizontalpodautoscaler/staging/thirdparty",
			Tier: "notify", MaxMessages: 3, Tail: duration(30 * time.Minute),
		},
		build: buildScaleFailureReplaced,
	}
}

func buildScaleFailureReplaced(c *cluster) {
	c.list(c.node("n1", "zone-a"))
	c.after(2 * time.Minute)
	message := "the HPA controller was unable to get the target's " +
		"current scale: deployments.apps \"thirdparty\" not found"
	hpa := clusterHPA(c, "staging", "thirdparty", "True",
		"ValidMetricFound", "")
	hpa.Spec.ScaleTargetRef.Kind = "Rollout"
	hpa.Status.Conditions[0] = autoscalingv2.
		HorizontalPodAutoscalerCondition{
		Type: autoscalingv2.AbleToScale, Status: "False",
		Reason: "FailedGetScale", Message: message,
		LastTransitionTime: metav1.NewTime(c.now),
	}
	c.list(hpa)
	c.after(30 * time.Minute)
	hpa.Spec.ScaleTargetRef.Kind = "Deployment"
	c.list(hpa)
	c.after(10 * time.Minute)
}
