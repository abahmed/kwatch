package detectors

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/upstream"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// These tests hold kwatch to the upstream snapshot: every reason and
// condition Kubernetes emits is either classified, or named below with
// the reason it is left alone. A reason added to the snapshot without
// either fails here, so a gap is found by CI and not in an incident.

var coverageNow = time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)

// ignoredEvents are the Warning events that are deliberately not a
// finding of their own, with why. Every other Warning event reason in
// the snapshot has a registry entry.
var ignoredEvents = map[string]string{}

// ignoredConditions are conditions whose bad status raises no finding
// by itself, with why. Every other condition of the snapshot raises one.
var ignoredConditions = map[string]string{
	"Job/FailureTarget": "the Job controller sets it while the pods " +
		"stop and then sets Failed, which is a finding (job_finished.go)",
	"HorizontalPodAutoscaler/ScalingLimited": "limited is normal; it is " +
		"HPAMaxedOut only when the autoscaler is also at its maximum " +
		"and wants more (hpa.go)",
	"CustomResourceDefinition/NonStructuralSchema": "only legacy " +
		"v1beta1 CRDs; the API is still served, so nothing fails",
	"CustomResourceDefinition/KubernetesAPIApprovalPolicyConformant": "" +
		"advisory about the approval annotation of a *.k8s.io group; " +
		"the API is still served",
}

func TestEveryUpstreamWaitingReasonIsClassified(t *testing.T) {
	for _, reason := range upstream.WaitingReasons {
		_, failing := detection.WaitingSeverity(reason)
		assert.True(t, failing || detection.WaitingProgress(reason),
			"waiting reason %q is neither a failure nor progress", reason)
	}
}

// A terminated container that exited non-zero is a finding whatever its
// reason; Completed with exit code zero is the one that is not.
func TestEveryUpstreamTerminatedReasonIsClassified(t *testing.T) {
	for _, reason := range upstream.TerminatedReasons {
		code := 1.0
		if reason == "Completed" {
			code = 0
		}
		c := buildContainer(
			inventory.EntityID{Kind: kube.KindPod, Namespace: "ns",
				Name: "p"}, "app", coverageNow, map[string]inventory.Value{
				kube.AttrState:       inventory.Text("terminated"),
				kube.AttrStateReason: inventory.Text(reason),
				kube.AttrExitCode:    inventory.Number(code),
			})
		found := Container{}.Detect(testDetectorContext(nil, coverageNow), c)
		if code == 0 {
			assert.Empty(t, found, reason)
			continue
		}
		assert.NotEmpty(t, found, "terminated reason %q is ignored", reason)
	}
}

// A failed pod is a finding whatever its status reason; the reasons
// that name why get a finding of their own.
func TestEveryUpstreamPodReasonIsClassified(t *testing.T) {
	for _, reason := range upstream.PodReasons {
		pod := buildPod("p", "ns", coverageNow, map[string]inventory.Value{
			kube.AttrPhase:  inventory.Text("Failed"),
			kube.AttrReason: inventory.Text(reason),
		})
		found := NewPod(PodThresholds{}).Detect(
			testDetectorContext(nil, coverageNow), pod)
		assert.NotEmpty(t, found, "failed pod reason %q is ignored", reason)
	}
}

func TestEveryUpstreamWarningEventIsClassified(t *testing.T) {
	for _, event := range upstream.Events {
		if !event.Warning {
			continue
		}
		_, failing := detection.EventSeverity(event.Reason)
		classified := failing || detection.ShownByState(event.Reason) ||
			detection.KnownReason(event.Reason)
		_, ignored := ignoredEvents[event.Reason]
		assert.True(t, classified != ignored,
			"Warning event %q: classified=%v ignored=%v",
			event.Reason, classified, ignored)
	}
}

// conditionEntity builds an entity of the kind with the condition at its
// trouble status for an hour, and the other state the kind's detectors
// read next to it (a pod's phase and readiness, a workload's replicas).
func conditionEntity(
	c upstream.Condition, since time.Time,
) (*inventory.Model, inventory.EntityID) {
	m := newTestModel()
	kind := inventory.Kind(strings.ToLower(c.Kind))
	id := inventory.EntityID{Kind: kind, Name: "x"}
	if kind != kube.KindNode && kind != kindCRD {
		id.Namespace = "ns"
	}
	attrs := map[string]inventory.Value{}
	switch c.Kind + "/" + c.Type {
	case "Pod/PodScheduled":
		attrs[kube.AttrPhase] = inventory.Text("Pending")
	case "Pod/Initialized", "Pod/PodReadyToStartContainers":
		attrs[kube.AttrPhase] = inventory.Text("Pending")
		attrs[kube.AttrStartTime] = inventory.Time(since)
	case "Pod/ContainersReady", "Pod/Ready":
		attrs[kube.AttrPhase] = inventory.Text("Running")
		attrs[kube.AttrReadySince] = inventory.Time(since)
	case "Deployment/Available":
		attrs[kube.AttrReplicas] = inventory.Number(3)
		attrs[kube.AttrDesiredReplicas] = inventory.Number(3)
		attrs[kube.AttrReadyReplicas] = inventory.Number(0)
	}
	m.Apply(inventory.Observation{Kind: inventory.Observed, Source: "test",
		At: since, Entity: id, Attributes: attrs})
	if kind == kube.KindPod && c.Type != "PodScheduled" {
		setCondition(m, id, "PodScheduled", "True", "", "", since)
	}
	setCondition(m, id, c.Type, c.Trouble, c.Reason, "", since)
	return m, id
}

func TestEveryUpstreamConditionIsClassified(t *testing.T) {
	for _, c := range upstream.Conditions {
		registry := detection.NewRegistry(nil, Default()...)
		key := c.Kind + "/" + c.Type
		if c.Trouble == "" {
			continue
		}
		m, id := conditionEntity(c, coverageNow.Add(-time.Hour))
		got := registry.Evaluate(m, coverageNow, id).Findings
		_, ignored := ignoredConditions[key]
		// The condition table is read by the kind's own detector, which
		// the plain registry may not reach (a CRD's, for one).
		tabled := slices.ContainsFunc(kindConditions[id.Kind],
			func(check conditionCheck) bool { return check.cond == c.Type })
		assert.True(t, ignored == (len(got) == 0 && !tabled),
			"condition %s: findings=%d ignored=%v", key, len(got), ignored)
	}
}

// An ignore entry that names nothing in the snapshot is stale.
func TestIgnoreListsNameSnapshotEntries(t *testing.T) {
	events := map[string]bool{}
	for _, e := range upstream.Events {
		events[e.Reason] = true
	}
	for reason := range ignoredEvents {
		assert.True(t, events[reason], reason)
	}
	conditions := map[string]bool{}
	for _, c := range upstream.Conditions {
		conditions[c.Kind+"/"+c.Type] = true
	}
	for key := range ignoredConditions {
		assert.True(t, conditions[key], key)
	}
}
