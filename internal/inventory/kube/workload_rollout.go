package kube

import (
	appsv1 "k8s.io/api/apps/v1"

	"github.com/abahmed/kwatch/internal/inventory"
)

// StatefulSet rollout attributes. AttrRevision holds the update revision;
// pods still on AttrCurrentRevision have not been replaced yet.
const (
	AttrCurrentRevision = "revision.current"
	AttrPartition       = "rollout.partition"
	AttrUpdateStrategy  = "update.strategy"
	AttrPodManagement   = "pod.management"
)

// statefulSetRollout records what a stalled StatefulSet rollout is
// judged by: both revisions, how many pods run each, the partition that
// holds ordinals back, and the strategy (OnDelete never rolls on its own).
func statefulSetRollout(
	s *appsv1.StatefulSet, attrs map[string]inventory.Value,
) {
	attrs[AttrCurrentRevision] = inventory.Text(s.Status.CurrentRevision)
	attrs[AttrCurrentReplicas] = number(s.Status.CurrentReplicas)
	strategy := s.Spec.UpdateStrategy
	if strategy.Type == "" {
		strategy.Type = appsv1.RollingUpdateStatefulSetStrategyType
	}
	attrs[AttrUpdateStrategy] = inventory.Text(string(strategy.Type))
	partition := int32(0)
	if rolling := strategy.RollingUpdate; rolling != nil &&
		rolling.Partition != nil {
		partition = *rolling.Partition
	}
	attrs[AttrPartition] = number(partition)
	management := s.Spec.PodManagementPolicy
	if management == "" {
		management = appsv1.OrderedReadyPodManagement
	}
	attrs[AttrPodManagement] = inventory.Text(string(management))
}
