package kube

import (
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"

	"github.com/abahmed/kwatch/internal/knowledge"
)

func number(value int32) knowledge.Value {
	return knowledge.Number(float64(value))
}

func deploymentStatus(d *appsv1.Deployment, attrs map[string]knowledge.Value) {
	st := d.Status
	attrs[AttrObservedGen] = knowledge.Number(float64(st.ObservedGeneration))
	attrs[AttrReadyReplicas] = number(st.ReadyReplicas)
	attrs[AttrAvailable] = number(st.AvailableReplicas)
	attrs[AttrUpdatedReplicas] = number(st.UpdatedReplicas)
	attrs[AttrUnavailable] = number(st.UnavailableReplicas)
	conditions := make([]condition, 0, len(st.Conditions))
	for _, c := range st.Conditions {
		conditions = append(conditions, condition{
			Type: string(c.Type), Status: string(c.Status),
			Reason: c.Reason, Since: c.LastTransitionTime.Time,
		})
	}
	setConditions(attrs, conditions)
}

func replicaSetStatus(r *appsv1.ReplicaSet, attrs map[string]knowledge.Value) {
	st := r.Status
	attrs[AttrObservedGen] = knowledge.Number(float64(st.ObservedGeneration))
	attrs[AttrReadyReplicas] = number(st.ReadyReplicas)
	attrs[AttrAvailable] = number(st.AvailableReplicas)
	const revisionAnnotation = "deployment.kubernetes.io/revision"
	if revision := r.Annotations[revisionAnnotation]; revision != "" {
		attrs[AttrRevision] = knowledge.Text(revision)
	}
	conditions := make([]condition, 0, len(st.Conditions))
	for _, c := range st.Conditions {
		conditions = append(conditions, condition{
			Type: string(c.Type), Status: string(c.Status),
			Reason: c.Reason, Since: c.LastTransitionTime.Time,
		})
	}
	setConditions(attrs, conditions)
}

func statefulSetStatus(
	s *appsv1.StatefulSet, attrs map[string]knowledge.Value,
) {
	st := s.Status
	attrs[AttrObservedGen] = knowledge.Number(float64(st.ObservedGeneration))
	attrs[AttrReadyReplicas] = number(st.ReadyReplicas)
	attrs[AttrAvailable] = number(st.AvailableReplicas)
	attrs[AttrUpdatedReplicas] = number(st.UpdatedReplicas)
	attrs[AttrRevision] = knowledge.Text(st.UpdateRevision)
	conditions := make([]condition, 0, len(st.Conditions))
	for _, c := range st.Conditions {
		conditions = append(conditions, condition{
			Type: string(c.Type), Status: string(c.Status),
			Reason: c.Reason, Since: c.LastTransitionTime.Time,
		})
	}
	setConditions(attrs, conditions)
}

func daemonSetStatus(d *appsv1.DaemonSet, attrs map[string]knowledge.Value) {
	st := d.Status
	attrs[AttrObservedGen] = knowledge.Number(float64(st.ObservedGeneration))
	attrs[AttrDesiredReplicas] = number(st.DesiredNumberScheduled)
	attrs[AttrReadyReplicas] = number(st.NumberReady)
	attrs[AttrAvailable] = number(st.NumberAvailable)
	attrs[AttrUpdatedReplicas] = number(st.UpdatedNumberScheduled)
	attrs[AttrUnavailable] = number(st.NumberUnavailable)
}

func jobStatus(j *batchv1.Job, attrs map[string]knowledge.Value) {
	st := j.Status
	attrs[AttrActive] = number(st.Active)
	attrs[AttrSucceeded] = number(st.Succeeded)
	attrs[AttrFailed] = number(st.Failed)
	if j.Spec.BackoffLimit != nil {
		attrs[AttrBackoffLimit] = number(*j.Spec.BackoffLimit)
	}
	attrs[AttrSuspended] = knowledge.Bool(
		j.Spec.Suspend != nil && *j.Spec.Suspend,
	)
	if st.StartTime != nil {
		attrs[AttrStartTime] = knowledge.Time(st.StartTime.Time)
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
