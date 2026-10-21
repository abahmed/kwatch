package detectors

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A rollout check is registered only for a kind whose status has the
// condition: a StatefulSet and a DaemonSet have neither Progressing nor
// ReplicaFailure, so a row for them could never fire.
func TestConditionTableHasNoRolloutRowsForKindsWithoutThoseConditions(
	t *testing.T,
) {
	assert.NotEmpty(t, kindConditions[kube.KindDeployment])
	assert.NotEmpty(t, kindConditions[kube.KindReplicaSet])
	assert.Empty(t, kindConditions[kube.KindStatefulSet])
	assert.Empty(t, kindConditions[kube.KindDaemonSet])
}
