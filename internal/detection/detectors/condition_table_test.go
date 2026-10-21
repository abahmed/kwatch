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

// condCase is one condition on one kind and what the detectors report
// for it. The cases were taken from the per-kind detectors the condition
// table replaced, so the table is held to their exact output.
type condCase struct {
	name    string
	kind    inventory.Kind
	cond    string
	status  string
	reason  string
	message string
	age     time.Duration
	// extra are further conditions set on the same entity.
	extra []condSpec
	want  []wantFinding
}

type condSpec struct {
	cond, status, reason string
	age                  time.Duration
}

type wantFinding struct {
	reason   string
	severity detection.Severity
	health   detection.Health
	summary  string
	// message is the quoted condition message; noMessage means no
	// evidence at all.
	message   *string
	noMessage bool
}

func msg(s string) *string { return &s }

var condCases = []condCase{
	{name: "memory pressure held", kind: kube.KindNode,
		cond: "MemoryPressure", status: "True", age: 2 * time.Minute,
		want: []wantFinding{{reasons.MemoryPressure, detection.Critical, 0,
			"Node is low on memory", nil, true}}},
	{name: "memory pressure blip", kind: kube.KindNode,
		cond: "MemoryPressure", status: "True", age: 10 * time.Second},
	{name: "disk pressure", kind: kube.KindNode,
		cond: "DiskPressure", status: "True", age: 2 * time.Minute,
		want: []wantFinding{{reasons.DiskPressure, detection.Critical, 0,
			"Node is low on disk space", nil, true}}},
	{name: "pid pressure", kind: kube.KindNode,
		cond: "PIDPressure", status: "True", age: 2 * time.Minute,
		want: []wantFinding{{reasons.PIDPressure, detection.Critical, 0,
			"Node is running out of process IDs", nil, true}}},
	{name: "pressure false", kind: kube.KindNode,
		cond: "DiskPressure", status: "False", age: 2 * time.Minute},
	{name: "network unavailable at once", kind: kube.KindNode,
		cond: "NetworkUnavailable", status: "True", reason: "NoRoute",
		age: time.Second,
		want: []wantFinding{{reasons.NetworkUnavailable, detection.Critical,
			0, "Node network is not configured (NoRoute)", nil, true}}},
	{name: "network unavailable without reason", kind: kube.KindNode,
		cond: "NetworkUnavailable", status: "True", age: time.Second,
		want: []wantFinding{{reasons.NetworkUnavailable, detection.Critical,
			0, "Node network is not configured ()", nil, true}}},
	{name: "progress deadline", kind: kube.KindDeployment,
		cond: "Progressing", status: "False",
		reason: reasons.ProgressDeadlineExceeded,
		want: []wantFinding{{reasons.ProgressDeadlineExceeded,
			detection.Critical, 0, "Rollout is stuck: new pods did not " +
				"become available within the progress deadline", nil,
			true}}},
	{name: "progressing normally", kind: kube.KindDeployment,
		cond: "Progressing", status: "True", reason: "NewReplicaSetAvailable"},
	{name: "deployment replica failure", kind: kube.KindDeployment,
		cond: "ReplicaFailure", status: "True", reason: "FailedCreate",
		message: "exceeded quota",
		want: []wantFinding{{reasons.DeploymentReplicaFailure,
			detection.Critical, 0,
			"Controller cannot create pods (FailedCreate)",
			msg("exceeded quota"), false}}},
	{name: "replica failure without message", kind: kube.KindDeployment,
		cond: "ReplicaFailure", status: "True", reason: "FailedCreate",
		want: []wantFinding{{reasons.DeploymentReplicaFailure,
			detection.Critical, 0,
			"Controller cannot create pods (FailedCreate)", msg(""),
			false}}},
	{name: "replicaset failure", kind: kube.KindReplicaSet,
		cond: "ReplicaFailure", status: "True", reason: "FailedCreate",
		message: "denied",
		want: []wantFinding{{reasons.ReplicaSetFailure,
			detection.Critical, 0,
			"Controller cannot create pods (FailedCreate)", msg("denied"),
			false}}},
	{name: "replicaset progress deadline", kind: kube.KindReplicaSet,
		cond: "Progressing", status: "False",
		reason: reasons.ProgressDeadlineExceeded,
		want: []wantFinding{{reasons.ReplicaSetFailure, detection.Critical,
			0, "Rollout is stuck: new pods did not become available " +
				"within the progress deadline", nil, true}}},
	{name: "budget sync failed", kind: kube.KindPDB,
		cond: "DisruptionAllowed", status: "False", reason: "SyncFailed",
		message: "cannot count", age: 5 * time.Minute,
		want: []wantFinding{{reasons.PdbSyncFailed, detection.Warning,
			detection.Failing, "Disruption controller cannot evaluate " +
				"the budget; it allows no evictions",
			msg("cannot count"), false}}},
	{name: "budget sync failed too soon", kind: kube.KindPDB,
		cond: "DisruptionAllowed", status: "False", reason: "SyncFailed",
		age: time.Minute},
	{name: "budget other reason", kind: kube.KindPDB,
		cond: "DisruptionAllowed", status: "False",
		reason: "InsufficientPods", age: 5 * time.Minute},
	{name: "crd names conflict", kind: "customresourcedefinition",
		cond: "NamesAccepted", status: "False", reason: "NameConflict",
		message: "taken", age: 5 * time.Minute,
		want: []wantFinding{{reasons.CRDNotEstablished, detection.Warning,
			detection.Failing, "Custom resource API is not served: " +
				"NamesAccepted=False (NameConflict)", msg("taken"),
			false}}},
	{name: "crd first listed wins", kind: "customresourcedefinition",
		cond: "NamesAccepted", status: "False", reason: "NameConflict",
		age: 5 * time.Minute,
		extra: []condSpec{
			{"Established", "False", "", 5 * time.Minute}},
		want: []wantFinding{{reasons.CRDNotEstablished, detection.Warning,
			detection.Failing, "Custom resource API is not served: " +
				"NamesAccepted=False (NameConflict)", msg(""), false}}},
	{name: "crd falls to the sustained one", kind: "customresourcedefinition",
		cond: "NamesAccepted", status: "False", age: time.Minute,
		extra: []condSpec{
			{"Established", "False", "", 5 * time.Minute}},
		want: []wantFinding{{reasons.CRDNotEstablished, detection.Warning,
			detection.Failing, "Custom resource API is not served: " +
				"Established=False", msg(""), false}}},
	{name: "claim volume modification error", kind: kube.KindPVC,
		cond: "ModifyVolumeError", status: "True", reason: "Boom",
		message: "class refused",
		want: []wantFinding{{reasons.PersistentVolumeClaim,
			detection.Warning, 0, "Volume modification failed (Boom)",
			msg("class refused"), false}}},
	{name: "claim resize error", kind: kube.KindPVC,
		cond: "ControllerResizeError", status: "True", reason: "Boom",
		message: "no space",
		want: []wantFinding{{reasons.PersistentVolumeClaim,
			detection.Warning, 0, "Volume resize failed (Boom)",
			msg("no space"), false}}},
	{name: "claim resize error, first listed wins", kind: kube.KindPVC,
		cond: "NodeResizeError", status: "True", reason: "Late",
		extra: []condSpec{{"ControllerResizeError", "True", "Early", 0}},
		want: []wantFinding{{reasons.PersistentVolumeClaim,
			detection.Warning, 0, "Volume resize failed (Early)",
			msg(""), false}}},
	{name: "pod resize infeasible", kind: kube.KindPod,
		cond: "PodResizePending", status: "True", reason: "Infeasible",
		message: "no room",
		want: []wantFinding{{reasons.PodResizeInfeasible,
			detection.Warning, 0, "Pod resize cannot be done on its node",
			msg("no room"), false}}},
	{name: "pod resize infeasible without message", kind: kube.KindPod,
		cond: "PodResizePending", status: "True", reason: "Infeasible",
		want: []wantFinding{{reasons.PodResizeInfeasible,
			detection.Warning, 0, "Pod resize cannot be done on its node",
			nil, true}}},
	{name: "pod resize deferred waits", kind: kube.KindPod,
		cond: "PodResizePending", status: "True", reason: "Deferred",
		age: time.Minute},
	{name: "pod resize deferred", kind: kube.KindPod,
		cond: "PodResizePending", status: "True", reason: "Deferred",
		age: 6 * time.Minute,
		want: []wantFinding{{reasons.PodResizeDeferred, detection.Warning,
			0, "Pod resize has been deferred for lack of room on its node",
			nil, true}}},
	{name: "pod resize error", kind: kube.KindPod,
		cond: "PodResizeInProgress", status: "True", reason: "Error",
		message: "kubelet said no",
		want: []wantFinding{{reasons.PodResizeError, detection.Warning, 0,
			"Pod resize failed to apply", msg("kubelet said no"), false}}},
}

var tableReasons = map[string]bool{
	reasons.MemoryPressure: true, reasons.DiskPressure: true,
	reasons.PIDPressure: true, reasons.NetworkUnavailable: true,
	reasons.ProgressDeadlineExceeded: true,
	reasons.DeploymentReplicaFailure: true, reasons.ReplicaSetFailure: true,
	reasons.PdbSyncFailed: true, reasons.CRDNotEstablished: true,
	reasons.PersistentVolumeClaim: true, reasons.PodResizeInfeasible: true,
	reasons.PodResizeDeferred: true, reasons.PodResizeError: true,
}

// detectFor runs the detector that owns the kind.
func detectFor(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	switch e.ID.Kind {
	case kube.KindNode:
		return NewNode(0).Detect(ctx, e)
	case kube.KindPDB:
		return Budget{}.Detect(ctx, e)
	case kube.KindPVC:
		return Claim{}.Detect(ctx, e)
	case kube.KindPod:
		return NewPod(PodThresholds{}).Detect(ctx, e)
	case "customresourcedefinition":
		return Custom{}.Detect(ctx, e)
	}
	return NewWorkload(0).Detect(ctx, e)
}

func TestConditionTableReportsWhatTheDetectorsDid(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	for _, tc := range condCases {
		t.Run(tc.name, func(t *testing.T) {
			model := newTestModel()
			id := inventory.EntityID{Kind: tc.kind, Name: "x"}
			if tc.kind != kube.KindNode {
				id.Namespace = "default"
			}
			attrs := map[string]inventory.Value{}
			put := func(cond, status, reason, message string,
				age time.Duration,
			) {
				key := kube.ConditionKey(cond)
				attrs[key] = inventory.Text(status)
				attrs[key+kube.AttrConditionReason] = inventory.Text(reason)
				if message != "" {
					attrs[key+kube.AttrConditionMessage] =
						inventory.Text(message)
				}
				attrs[key+kube.AttrConditionSince] =
					inventory.Time(now.Add(-age))
			}
			put(tc.cond, tc.status, tc.reason, tc.message, tc.age)
			for _, x := range tc.extra {
				put(x.cond, x.status, x.reason, "", x.age)
			}
			switch tc.kind {
			case kube.KindPod:
				attrs[kube.AttrPhase] = inventory.Text("Running")
			case kube.KindPVC:
				attrs[kube.AttrPhase] = inventory.Text("Bound")
			case "customresourcedefinition":
				attrs[kube.AttrCustom] = inventory.Bool(true)
			}
			observeEntity(model, id, now.Add(-time.Hour), attrs)
			var got []detection.Finding
			for _, f := range detectFor(testDetectorContext(model, now),
				entityOf(model, id)) {
				if tableReasons[f.Reason] {
					got = append(got, detection.Classify(f))
				}
			}
			require.Len(t, got, len(tc.want))
			for i, w := range tc.want {
				f := got[i]
				assert.Equal(t, w.reason, f.Reason)
				assert.Equal(t, w.severity, f.Severity)
				if w.health != 0 {
					assert.Equal(t, w.health, f.Health)
				}
				assert.Equal(t, w.summary, f.Summary)
				if w.noMessage {
					assert.Empty(t, f.Evidence)
					continue
				}
				require.Len(t, f.Evidence, 1)
				assert.Equal(t, "message", f.Evidence[0].Label)
				assert.Equal(t, *w.message, f.Evidence[0].Value)
			}
		})
	}
}
