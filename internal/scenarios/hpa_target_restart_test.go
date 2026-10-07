package scenarios

import (
	"context"
	"testing"
	"time"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/replay"
)

// agedMissingTargetHPA is an autoscaler whose Deployment was deleted
// long ago: the controller's AbleToScale condition has said
// FailedGetScale for two years, as on a staging cluster.
func agedMissingTargetHPA(
	c *cluster,
) *autoscalingv2.HorizontalPodAutoscaler {
	message := "the HPA controller was unable to get the target's " +
		"current scale: deployments.apps \"thirdparty\" not found"
	hpa := clusterHPA(c, "staging", "thirdparty", "True",
		"ValidMetricFound", "")
	hpa.Status.Conditions[0] = autoscalingv2.
		HorizontalPodAutoscalerCondition{
		Type: autoscalingv2.AbleToScale, Status: "False",
		Reason: "FailedGetScale", Message: message,
		LastTransitionTime: metav1.NewTime(c.now.AddDate(-2, 0, 0)),
	}
	return hpa
}

// hpaTargetMembers lists the reasons the incident of the autoscaler
// holds, or nil when no incident has it as its root.
func hpaTargetMembers(result replay.Result) map[string]bool {
	for _, p := range result.Incidents {
		if p.Root.Kind != kube.KindHPA || p.Root.Name != "thirdparty" {
			continue
		}
		reasons := map[string]bool{}
		for member := range p.Members {
			reasons[member.Reason] = true
		}
		return reasons
	}
	return nil
}

// An autoscaler whose Deployment was deleted long ago is found even
// when its list reached kwatch before the Deployments did.
func TestAgedMissingScaleTargetIsFoundWhenDeploymentsSyncLate(
	t *testing.T,
) {
	deps := newDependencies()
	model := deps.Model
	synced := func(kind inventory.Kind) bool {
		return kind != kube.KindDeployment ||
			len(model.Entities(kube.KindDeployment)) > 0
	}
	deps.Synced = synced
	deps.Detectors = detection.NewRegistry(synced, appDetectors()...)
	c := newCluster(scenarioStart, "")
	c.list(c.node("n1", "zone-a"))
	c.list(agedMissingTargetHPA(c))
	c.after(2 * time.Second)
	other := c.deployment("staging", "other", "registry.example.com/o:1", 1)
	c.list(other.objects())
	c.after(20 * time.Minute)
	log := c.log()

	result, err := replay.Run(context.Background(), log, deps,
		replay.Options{SyncAt: log.Start.Add(3 * time.Second),
			Tail: 40 * time.Minute})
	if err != nil {
		t.Fatal(err)
	}

	if !hpaTargetMembers(result)["HPATargetMissing"] {
		t.Errorf("no missing-target finding: %s", describeDecisions(result))
	}
}

// A restart over an incident an older release opened for the same
// autoscaler (it read the failure as FailedGetScale) ends with that
// incident holding the missing-target finding, and the restart listing
// names it.
func TestRestartMovesOldScaleFailureToMissingTarget(t *testing.T) {
	store := newPersistingStore()
	c := newCluster(scenarioStart, "")
	c.list(c.node("n1", "zone-a"))
	hpa := agedMissingTargetHPA(c)
	// The older release did not know a Rollout is missing: it reported
	// the controller's own FailedGetScale condition.
	hpa.Spec.ScaleTargetRef.Kind = "Rollout"
	c.list(hpa)
	c.after(30 * time.Minute)
	log := c.log()
	first := restartReplay(t, log, store, replay.Options{
		SyncAt: log.Start, Tail: 30 * time.Minute})
	if got := hpaTargetMembers(first); !got["FailedGetScale"] {
		t.Fatalf("first session members = %v, want FailedGetScale", got)
	}

	restartAt := first.End.Add(restartGap)
	again := newCluster(scenarioStart, "")
	again.now = restartAt
	again.list(again.node("n1", "zone-a"))
	hpa.Spec.ScaleTargetRef.Kind = "Deployment"
	again.list(hpa)
	again.after(10 * time.Minute)
	log = again.log()
	log.Start = restartAt.UTC()
	second := restartReplay(t, log, store, replay.Options{
		SyncAt: restartAt, Tail: 30 * time.Minute})

	got := hpaTargetMembers(second)
	if !got["HPATargetMissing"] || got["FailedGetScale"] {
		t.Errorf("second session members = %v, want HPATargetMissing only",
			got)
	}
	listed := false
	for _, d := range second.Decisions {
		listed = listed || d.Reason == "restored incidents"
	}
	if !listed {
		t.Errorf("restart did not list the incident: %s",
			describeDecisions(second))
	}
	if !saidCauseRevised(second) {
		t.Errorf("the thread never heard the new cause: %s",
			describeDecisions(second))
	}
	for _, p := range second.Incidents {
		if p.Root.Name == "thirdparty" && p.Tier != incident.Digest {
			t.Errorf("tier = %v, want digest", p.Tier)
		}
	}
}

// saidCauseRevised reports that some incident's thread was told its
// cause was revised.
func saidCauseRevised(result replay.Result) bool {
	for _, d := range result.Decisions {
		if d.Action == incident.Update &&
			d.Reason == incident.ReasonCauseRevised {
			return true
		}
	}
	return false
}

// An incident opened at the notify tier from the controller's own
// FailedGetScale words, whose finding is later replaced by the clearer
// missing-target one (the scale target kind became known), falls to the
// digest and tells its thread the cause once.
func TestNotifyScaleFailureReplacedByMissingTargetSaysSo(t *testing.T) {
	c := newCluster(scenarioStart, "")
	c.list(c.node("n1", "zone-a"))
	hpa := agedMissingTargetHPA(c)
	hpa.Spec.ScaleTargetRef.Kind = "Rollout"
	c.list(hpa)
	c.after(30 * time.Minute)
	hpa.Spec.ScaleTargetRef.Kind = "Deployment"
	c.list(hpa)
	c.after(30 * time.Minute)
	log := c.log()

	result, err := replay.Run(context.Background(), log, newDependencies(),
		replay.Options{SyncAt: log.Start, Tail: 40 * time.Minute})
	if err != nil {
		t.Fatal(err)
	}

	if got := hpaTargetMembers(result); !got["HPATargetMissing"] ||
		got["FailedGetScale"] {
		t.Errorf("members = %v, want HPATargetMissing only", got)
	}
	revised := 0
	for i, d := range result.Decisions {
		if d.Action == incident.Update &&
			d.Reason == incident.ReasonCauseRevised {
			revised++
			t.Logf("%s", result.Messages[i].Title)
		}
	}
	if revised != 1 {
		t.Errorf("cause updates = %d, want 1: %s", revised,
			describeDecisions(result))
	}
}
