package handler

import (
	"github.com/abahmed/kwatch/internal/event"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

const (
	statefulSetReplicaFailure = "ReplicaFailure"
)

// DetectStatefulSetIssue returns a Signal if the StatefulSet has a ReplicaFailure
// condition or has fewer ready replicas than desired (unhealthy).
// Used for baseline seeding at startup.
func DetectStatefulSetIssue(ss *appsv1.StatefulSet) *event.Signal {
	for _, c := range ss.Status.Conditions {
		if string(c.Type) == statefulSetReplicaFailure && c.Status == corev1.ConditionTrue {
			return &event.Signal{
				Resource:  "statefulset",
				Reason:    "StatefulSetReplicaFailure",
				Namespace: ss.Namespace,
				Owner:     ss.Namespace + "/" + ss.Name,
				Labels:    ss.Labels,
				Message:   c.Message,
			}
		}
	}
	if ss.Status.Replicas > 0 && ss.Status.ReadyReplicas < ss.Status.Replicas {
		msg := ""
		if len(ss.Status.Conditions) > 0 {
			msg = ss.Status.Conditions[0].Message
		}
		return &event.Signal{
			Resource:  "statefulset",
			Reason:    "StatefulSetUnhealthy",
			Namespace: ss.Namespace,
			Owner:     ss.Namespace + "/" + ss.Name,
			Labels:    ss.Labels,
			Message:   msg,
		}
	}
	return nil
}
