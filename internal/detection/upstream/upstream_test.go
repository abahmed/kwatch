package upstream

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"

	"github.com/stretchr/testify/assert"
)

func names[T ~string](in ...T) []string {
	out := make([]string, 0, len(in))
	for _, v := range in {
		out = append(out, string(v))
	}
	return out
}

// The typed constants k8s.io/api exports must all be in the snapshot,
// so a release that adds one fails here until the snapshot follows.
func TestSnapshotHoldsTheModuleConstants(t *testing.T) {
	phases := names(corev1.PodPending, corev1.PodRunning,
		corev1.PodSucceeded, corev1.PodFailed, corev1.PodUnknown)
	assert.ElementsMatch(t, phases, PodPhases)

	reasons := names(corev1.PodReasonUnschedulable,
		corev1.PodReasonSchedulingGated, corev1.PodReasonSchedulerError,
		corev1.PodReasonPreemptionByScheduler)
	assert.Subset(t, PodConditionReasons, reasons)

	conditions := map[string][]string{
		"Deployment": names(appsv1.DeploymentAvailable,
			appsv1.DeploymentProgressing, appsv1.DeploymentReplicaFailure),
		"Node": names(corev1.NodeReady, corev1.NodeMemoryPressure,
			corev1.NodeDiskPressure, corev1.NodePIDPressure,
			corev1.NodeNetworkUnavailable),
		"Pod": names(corev1.PodScheduled, corev1.PodInitialized,
			corev1.ContainersReady, corev1.PodReady,
			corev1.DisruptionTarget, corev1.PodReadyToStartContainers),
		"PersistentVolumeClaim": names(corev1.PersistentVolumeClaimResizing,
			corev1.PersistentVolumeClaimFileSystemResizePending,
			corev1.PersistentVolumeClaimControllerResizeError,
			corev1.PersistentVolumeClaimNodeResizeError,
			corev1.PersistentVolumeClaimVolumeModifyVolumeError,
			corev1.PersistentVolumeClaimVolumeModifyingVolume),
		"Job": names(batchv1.JobComplete, batchv1.JobFailed,
			batchv1.JobSuspended, batchv1.JobFailureTarget,
			batchv1.JobSuccessCriteriaMet),
		"HorizontalPodAutoscaler": names(autoscalingv2.AbleToScale,
			autoscalingv2.ScalingActive, autoscalingv2.ScalingLimited),
	}
	for kind, want := range conditions {
		var have []string
		for _, c := range Conditions {
			if c.Kind == kind {
				have = append(have, c.Type)
			}
		}
		assert.Subset(t, have, want, kind)
	}
}

func TestSnapshotHasNoDuplicates(t *testing.T) {
	for name, list := range map[string][]string{
		"waiting": WaitingReasons, "terminated": TerminatedReasons,
		"pod": PodReasons, "podCondition": PodConditionReasons,
	} {
		assert.Equal(t, len(list), len(set(list)), name)
	}
	var events []string
	for _, e := range Events {
		events = append(events, e.Reason+"/"+boolWord(e.Warning))
	}
	assert.Equal(t, len(events), len(set(events)), "events")
	var conds []string
	for _, c := range Conditions {
		conds = append(conds, c.Kind+"/"+c.Type)
	}
	assert.Equal(t, len(conds), len(set(conds)), "conditions")
}

func boolWord(b bool) string {
	if b {
		return "warning"
	}
	return "normal"
}

func set(in []string) map[string]bool {
	out := map[string]bool{}
	for _, v := range in {
		out[v] = true
	}
	return out
}
