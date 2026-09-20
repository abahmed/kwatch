// Package observe builds observations from Kubernetes objects.
//
// Everything kwatch reports starts here. A producer says which object it is
// looking at and what it found; this package fills in the identity -- the
// resource word, the namespace, the pod UID and lineage, the labels, the node
// -- from the object itself. Producers used to fill those fields by hand at
// forty-odd sites, and the fields they missed were invisible: a silence rule
// that never matched, a replacement pod that opened a second incident.
package observe

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/model"
)

// PodLineageAnnotation lets an operator declare that a generated pod
// continues an earlier one, so correlation folds a replacement into the
// incident its predecessor opened instead of announcing it as new.
const PodLineageAnnotation = "kwatch.abahmed.dev/lineage-id"

// Pod is an observation about pod, or about one container in it when
// container is non-empty. Pass "." for a finding about the pod itself rather
// than any of its containers.
//
// owners resolves the workload the incident should be keyed by; a nil
// resolver, or one that cannot answer, leaves the pod as its own owner, which
// is what keeps independent ownerless pods from being folded together.
func Pod(
	pod *corev1.Pod, container, reason string, owners OwnerResolver,
) *model.Observation {
	var owner model.ObjectRef
	if owners != nil {
		owner = owners.OwnerOf(pod)
	}
	return PodOwnedBy(pod, container, reason, owner)
}

// PodOwnedBy is Pod for a caller that has already resolved the owner --
// the pod pipeline resolves it once and reuses it across several findings.
func PodOwnedBy(
	pod *corev1.Pod, container, reason string, owner model.ObjectRef,
) *model.Observation {
	obs := &model.Observation{
		Subject:   model.ObjectRef{Kind: "pod"},
		Owner:     owner,
		Container: container,
		Reason:    reason,
	}
	if pod == nil {
		return obs
	}
	obs.Subject.Namespace = pod.Namespace
	obs.Subject.Name = pod.Name
	obs.Pod = model.PodIdentity{
		UID:          string(pod.UID),
		GenerateName: pod.GenerateName,
	}
	if pod.Annotations != nil {
		obs.Pod.LineageID = pod.Annotations[PodLineageAnnotation]
	}
	obs.NodeName = pod.Spec.NodeName
	obs.Labels = pod.Labels
	if obs.Owner.Name == "" {
		obs.Owner = SelfOwner("Pod", pod.Namespace, pod.Name)
	}
	return obs
}

// Object is an observation about a namespaced object that is not a pod. Such
// an object owns itself: there is no workload above a Service or an Ingress
// that its incidents should be keyed by.
func Object(kind string, obj metav1.Object, reason string) *model.Observation {
	if obj == nil {
		return ObjectNamed(kind, "", "", reason)
	}
	return ObjectNamed(kind, obj.GetNamespace(), obj.GetName(), reason).
		WithLabels(obj.GetLabels())
}

// ObjectNamed is Object for a producer that has the object's coordinates but
// not the object -- an unstructured status watch, a name recovered from a
// cache key.
func ObjectNamed(
	kind, namespace, name, reason string,
) *model.Observation {
	return &model.Observation{
		Subject: model.ObjectRef{
			Kind: kind, Namespace: namespace, Name: name,
		},
		Owner:  SelfOwner(kind, namespace, name),
		Reason: reason,
	}
}

// VolumeUsage is an observation about a PersistentVolumeClaim, witnessed
// through the pod that has it mounted.
//
// The claim is the owner: usage belongs to the volume, not to whichever
// replica happens to mount it this hour. The subject keeps the pod's name so
// the alert can still say where the full volume was seen. The owner carries
// no Kind, because there is no workload above the claim to report.
func VolumeUsage(
	namespace, claim, pod, reason string,
) *model.Observation {
	obs := ObjectNamed("pvc", namespace, pod, reason)
	obs.Owner = model.ObjectRef{Namespace: namespace, Name: claim}
	return obs
}

// Namespace is an observation about a namespace.
//
// A namespace is the one subject whose namespace is itself, so its owner is
// its bare name: "prod", never "prod/prod". Incident keys carry that
// encoding, and changing it would orphan every persisted namespace incident.
func Namespace(ns *corev1.Namespace, reason string) *model.Observation {
	if ns == nil {
		return ObjectNamed("namespace", "", "", reason)
	}
	obs := ObjectNamed("namespace", ns.Name, ns.Name, reason)
	obs.Owner = model.ObjectRef{Kind: "Namespace", Name: ns.Name}
	return obs.WithLabels(ns.Labels)
}

// ClusterObject is an observation about a cluster-scoped object -- a Node, a
// Namespace, a PersistentVolume, a webhook configuration -- whose name is its
// whole identity.
func ClusterObject(kind, name, reason string) *model.Observation {
	return ObjectNamed(kind, "", name, reason)
}

// Node is an observation about a node. NodeName is filled in as well as the
// subject, because attribution reads it to decide which pod failures a node
// failure speaks for.
func Node(node *corev1.Node, reason string) *model.Observation {
	if node == nil {
		return NodeNamed("", reason)
	}
	obs := NodeNamed(node.Name, reason)
	return obs.WithLabels(node.Labels)
}

// NodeNamed is Node for a producer that has only the node's name.
func NodeNamed(name, reason string) *model.Observation {
	obs := ClusterObject("node", name, reason)
	obs.NodeName = name
	return obs
}

// Synthetic is an observation about something that is not a Kubernetes object
// at all: a control-plane endpoint, an active probe target, the cluster
// autoscaler. owner is the stable name its incidents are keyed by.
func Synthetic(kind, owner, reason string) *model.Observation {
	obs := ObjectNamed(kind, "", owner, reason)
	return obs
}

// SelfOwner is the owner reference for a subject that owns itself. The Kind
// is carried through unchanged so an object's own kind still reaches
// severityByOwnerKind.
func SelfOwner(kind, namespace, name string) model.ObjectRef {
	return model.ObjectRef{Kind: kind, Namespace: namespace, Name: name}
}
