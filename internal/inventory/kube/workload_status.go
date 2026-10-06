package kube

import (
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"

	"github.com/abahmed/kwatch/internal/inventory"
)

func number(value int32) inventory.Value {
	return inventory.Number(float64(value))
}

func deploymentStatus(d *appsv1.Deployment, attrs map[string]inventory.Value) {
	st := d.Status
	attrs[AttrObservedGen] = inventory.Number(float64(st.ObservedGeneration))
	attrs[AttrReadyReplicas] = number(st.ReadyReplicas)
	attrs[AttrAvailable] = number(st.AvailableReplicas)
	attrs[AttrUpdatedReplicas] = number(st.UpdatedReplicas)
	attrs[AttrUnavailable] = number(st.UnavailableReplicas)
	conditions := make([]condition, 0, len(st.Conditions))
	for _, c := range st.Conditions {
		conditions = append(conditions, condition{
			Type: string(c.Type), Status: string(c.Status),
			Reason: c.Reason, Message: c.Message,
			Since: c.LastTransitionTime.Time,
		})
	}
	setConditions(attrs, conditions)
}

func replicaSetStatus(r *appsv1.ReplicaSet, attrs map[string]inventory.Value) {
	st := r.Status
	attrs[AttrObservedGen] = inventory.Number(float64(st.ObservedGeneration))
	attrs[AttrReadyReplicas] = number(st.ReadyReplicas)
	attrs[AttrAvailable] = number(st.AvailableReplicas)
	const revisionAnnotation = "deployment.kubernetes.io/revision"
	if revision := r.Annotations[revisionAnnotation]; revision != "" {
		attrs[AttrRevision] = inventory.Text(revision)
	}
	conditions := make([]condition, 0, len(st.Conditions))
	for _, c := range st.Conditions {
		conditions = append(conditions, condition{
			Type: string(c.Type), Status: string(c.Status),
			Reason: c.Reason, Message: c.Message,
			Since: c.LastTransitionTime.Time,
		})
	}
	setConditions(attrs, conditions)
}

func statefulSetStatus(
	s *appsv1.StatefulSet, attrs map[string]inventory.Value,
) {
	st := s.Status
	attrs[AttrObservedGen] = inventory.Number(float64(st.ObservedGeneration))
	attrs[AttrReadyReplicas] = number(st.ReadyReplicas)
	attrs[AttrAvailable] = number(st.AvailableReplicas)
	attrs[AttrUpdatedReplicas] = number(st.UpdatedReplicas)
	attrs[AttrRevision] = inventory.Text(st.UpdateRevision)
	statefulSetRollout(s, attrs)
	conditions := make([]condition, 0, len(st.Conditions))
	for _, c := range st.Conditions {
		conditions = append(conditions, condition{
			Type: string(c.Type), Status: string(c.Status),
			Reason: c.Reason, Message: c.Message,
			Since: c.LastTransitionTime.Time,
		})
	}
	setConditions(attrs, conditions)
}

func daemonSetStatus(d *appsv1.DaemonSet, attrs map[string]inventory.Value) {
	st := d.Status
	attrs[AttrObservedGen] = inventory.Number(float64(st.ObservedGeneration))
	attrs[AttrDesiredReplicas] = number(st.DesiredNumberScheduled)
	attrs[AttrReadyReplicas] = number(st.NumberReady)
	attrs[AttrAvailable] = number(st.NumberAvailable)
	attrs[AttrUpdatedReplicas] = number(st.UpdatedNumberScheduled)
	attrs[AttrUnavailable] = number(st.NumberUnavailable)
}

func jobStatus(j *batchv1.Job, attrs map[string]inventory.Value) {
	st := j.Status
	attrs[AttrActive] = number(st.Active)
	attrs[AttrSucceeded] = number(st.Succeeded)
	attrs[AttrFailed] = number(st.Failed)
	if j.Spec.BackoffLimit != nil {
		attrs[AttrBackoffLimit] = number(*j.Spec.BackoffLimit)
	}
	attrs[AttrSuspended] = inventory.Bool(
		j.Spec.Suspend != nil && *j.Spec.Suspend,
	)
	if st.StartTime != nil {
		attrs[AttrStartTime] = inventory.Time(st.StartTime.Time)
	}
	if st.CompletionTime != nil {
		attrs[AttrCompletionTime] = inventory.Time(st.CompletionTime.Time)
	}
	if deadline := j.Spec.ActiveDeadlineSeconds; deadline != nil {
		attrs[AttrActiveDeadline] = inventory.Number(float64(*deadline))
	}
	conditions := make([]condition, 0, len(st.Conditions))
	for _, c := range st.Conditions {
		conditions = append(conditions, condition{
			Type: string(c.Type), Status: string(c.Status),
			Reason: c.Reason, Message: c.Message,
			Since: c.LastTransitionTime.Time,
		})
	}
	setConditions(attrs, conditions)
}
