package controller

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// rebuildFrom feeds a complete lister result into a per-object graph rebuild.
func rebuildFrom[T any](
	items []T,
	err error,
	logMsg string,
	rebuild func(interface{}),
) error {
	if err != nil {
		return fmt.Errorf("%s: %w", logMsg, err)
	}
	for i := range items {
		rebuild(items[i])
	}
	return nil
}

func rebuildCheckedFrom[T any](
	items []T,
	err error,
	logMsg string,
	rebuild func(T) error,
) error {
	if err != nil {
		return fmt.Errorf("%s: %w", logMsg, err)
	}
	for _, item := range items {
		if err := rebuild(item); err != nil {
			return fmt.Errorf("%s: %w", logMsg, err)
		}
	}
	return nil
}

// buildResourceGraph rebuilds graph nodes for every non-Pod resource type.
func (b *graphBuilder) buildResourceGraph() error {
	if b.graph == nil {
		return nil
	}
	if b.pvcLister != nil {
		items, err := b.pvcLister.PersistentVolumeClaims(
			metav1.NamespaceAll,
		).List(labels.Everything())
		if err := rebuildFrom(
			items, err, "list pvcs for graph build",
			b.rebuildPersistentVolumeClaim,
		); err != nil {
			return err
		}
	}
	if b.rsLister != nil {
		items, err := b.rsLister.ReplicaSets(
			metav1.NamespaceAll,
		).List(labels.Everything())
		if err := rebuildFrom(
			items, err, "list replicasets for graph build", b.rebuildReplicaSet,
		); err != nil {
			return err
		}
	}
	if b.jobLister != nil {
		items, err := b.jobLister.Jobs(metav1.NamespaceAll).List(labels.Everything())
		if err := rebuildFrom(
			items, err, "list jobs for graph build", b.rebuildJob,
		); err != nil {
			return err
		}
	}
	if err := b.buildServiceGraph(); err != nil {
		return err
	}
	if b.ingressLister != nil {
		items, err := b.ingressLister.Ingresses(
			metav1.NamespaceAll,
		).List(labels.Everything())
		if err := rebuildFrom(
			items, err, "list ingresses for graph build", b.rebuildIngress,
		); err != nil {
			return err
		}
	}
	if b.hpaLister != nil {
		items, err := b.hpaLister.HorizontalPodAutoscalers(
			metav1.NamespaceAll,
		).List(labels.Everything())
		if err := rebuildFrom(
			items, err, "list hpas for graph build",
			b.rebuildHorizontalPodAutoscaler,
		); err != nil {
			return err
		}
	}
	if b.netpolLister != nil {
		items, err := b.netpolLister.NetworkPolicies(
			metav1.NamespaceAll,
		).List(labels.Everything())
		if err := rebuildCheckedFrom(
			items, err, "build networkpolicy graph edges",
			b.rebuildNetworkPolicyChecked,
		); err != nil {
			return err
		}
	}
	if b.pdbLister != nil {
		items, err := b.pdbLister.PodDisruptionBudgets(
			metav1.NamespaceAll,
		).List(labels.Everything())
		if err := rebuildCheckedFrom(
			items, err, "build poddisruptionbudget graph edges",
			b.rebuildPodDisruptionBudgetChecked,
		); err != nil {
			return err
		}
	}
	if b.endpointSliceLister != nil {
		items, err := b.endpointSliceLister.EndpointSlices(
			metav1.NamespaceAll,
		).List(labels.Everything())
		if err := rebuildFrom(
			items, err, "list endpoint slices for graph build",
			b.rebuildEndpointSlice,
		); err != nil {
			return err
		}
	}
	return b.buildPersistentVolumeGraph()
}

func (b *graphBuilder) buildServiceGraph() error {
	if b.serviceLister == nil {
		return nil
	}
	items, err := b.serviceLister.Services(
		metav1.NamespaceAll,
	).List(labels.Everything())
	return rebuildCheckedFrom(
		items, err, "build service graph edges", b.rebuildServiceChecked,
	)
}

func (b *graphBuilder) buildPersistentVolumeGraph() error {
	if b.pvLister == nil {
		return nil
	}
	items, err := b.pvLister.List(labels.Everything())
	return rebuildCheckedFrom(
		items, err, "build persistentvolume graph edges",
		b.rebuildPersistentVolumeChecked,
	)
}
