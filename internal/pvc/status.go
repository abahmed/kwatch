package pvc

import (
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const stuckVolumeDeletionGrace = 10 * time.Minute

func volumeStuckTerminating(
	deletion *metav1.Time,
	finalizers []string,
	now time.Time,
) bool {
	return deletion != nil && len(finalizers) > 0 &&
		now.Sub(deletion.Time) >= stuckVolumeDeletionGrace
}

func pvcStatusFailure(phase corev1.PersistentVolumeClaimPhase) bool {
	return phase == corev1.ClaimPending || phase == corev1.ClaimLost
}

func pvcFailureCondition(status corev1.PersistentVolumeClaimStatus) string {
	for resourceName, resizeStatus := range status.AllocatedResourceStatuses {
		switch resizeStatus {
		case corev1.PersistentVolumeClaimControllerResizeInfeasible,
			corev1.PersistentVolumeClaimNodeResizeInfeasible:
			return fmt.Sprintf("%s=%s", resourceName, resizeStatus)
		}
	}
	if status.ModifyVolumeStatus != nil &&
		status.ModifyVolumeStatus.Status ==
			corev1.PersistentVolumeClaimModifyVolumeInfeasible {
		return fmt.Sprintf(
			"ModifyVolume=%s", status.ModifyVolumeStatus.Status,
		)
	}
	for _, condition := range status.Conditions {
		if condition.Status != corev1.ConditionTrue {
			continue
		}
		switch condition.Type {
		case corev1.PersistentVolumeClaimControllerResizeError,
			corev1.PersistentVolumeClaimNodeResizeError,
			corev1.PersistentVolumeClaimVolumeModifyVolumeError,
			corev1.PersistentVolumeClaimFileSystemResizePending:
			return joinStatusDetails(string(condition.Type), condition.Message)
		}
	}
	return ""
}

func joinStatusDetails(reason, message string) string {
	if reason == "" {
		return message
	}
	if message == "" {
		return reason
	}
	return reason + ": " + message
}

func pvStatusFailure(phase corev1.PersistentVolumePhase) bool {
	return phase == corev1.VolumeReleased || phase == corev1.VolumeFailed
}
