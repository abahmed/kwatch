package observe

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	appsv1lister "k8s.io/client-go/listers/apps/v1"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/model"
)

// OwnerResolver answers the one question every pod-shaped producer has to
// ask: which workload should this pod's incidents be keyed by?
//
// It exists as an interface so there is exactly one answer. Three packages
// each had their own walk up the owner chain, and they disagreed: the kubelet
// monitor keyed by "namespace/pod", the metrics monitor by the pod name and
// the pod pipeline by the owning Deployment, so one broken workload arrived
// as three unrelated alerts.
type OwnerResolver interface {
	OwnerOf(pod *corev1.Pod) model.ObjectRef
}

// PodOwners resolves pod ownership from the informer caches.
//
// A ReplicaSet is reported as its parent Deployment -- the ReplicaSet is an
// implementation detail of a rollout, and keying by it would open a new
// incident on every deploy. A DaemonSet or StatefulSet is reported as itself
// unless it in turn has an owner. A pod with no owner references is its own
// owner.
type PodOwners struct {
	RS appsv1lister.ReplicaSetLister
	DS appsv1lister.DaemonSetLister
	SS appsv1lister.StatefulSetLister
	// Client is the fallback for a lister that is not wired. Optional: with
	// no client and no lister, resolution fails rather than guessing.
	Client kubernetes.Interface
	// Ctx bounds the fallback API calls. Ignored when Client is nil.
	Ctx context.Context
}

// OwnerOf implements OwnerResolver.
//
// A zero ObjectRef means "could not resolve" -- a lister error, or a workload
// that has gone. Callers must treat it as "do nothing" and never guess: an
// invented owner is a wrong incident key, which is a duplicate alert now and
// an unresolvable incident later.
func (p PodOwners) OwnerOf(pod *corev1.Pod) model.ObjectRef {
	if pod == nil {
		return model.ObjectRef{}
	}
	if len(pod.OwnerReferences) == 0 {
		return SelfOwner("Pod", pod.Namespace, pod.Name)
	}
	owner, ok := controllerOwnerReference(pod.OwnerReferences)
	if !ok {
		return model.ObjectRef{}
	}
	parent, resolved := p.parentOf(pod.Namespace, owner)
	if !resolved {
		return model.ObjectRef{}
	}
	if parent != nil {
		owner = *parent
	}
	return model.ObjectRef{
		Kind:      owner.Kind,
		Namespace: pod.Namespace,
		Name:      owner.Name,
	}
}

// parentOf returns the owner reference of the workload that owns the pod, if
// it has one. resolved is false when the workload could not be read at all.
func (p PodOwners) parentOf(
	namespace string, owner metav1.OwnerReference,
) (*metav1.OwnerReference, bool) {
	var get func() (metav1.Object, error)
	switch owner.Kind {
	case "ReplicaSet":
		get = p.replicaSetGetter(namespace, owner.Name)
	case "DaemonSet":
		get = p.daemonSetGetter(namespace, owner.Name)
	case "StatefulSet":
		get = p.statefulSetGetter(namespace, owner.Name)
	default:
		// Anything else (Job, a custom controller) is already the workload
		// an operator thinks in terms of.
		return nil, true
	}
	if get == nil {
		return nil, false
	}
	obj, err := get()
	if err != nil {
		klog.ErrorS(err, "owner resolve failed",
			"kind", owner.Kind, "name", owner.Name, "namespace", namespace)
		return nil, false
	}
	if refs := obj.GetOwnerReferences(); len(refs) > 0 {
		parent, ok := controllerOwnerReference(refs)
		if ok {
			return &parent, true
		}
	}
	return nil, true
}

// controllerOwnerReference selects the Kubernetes controller owner. A single
// unmarked owner remains a compatibility case for older or hand-built objects;
// multiple unmarked owners are ambiguous and must not be guessed.
func controllerOwnerReference(
	refs []metav1.OwnerReference,
) (metav1.OwnerReference, bool) {
	var controller *metav1.OwnerReference
	for i := range refs {
		ref := &refs[i]
		if ref.Controller == nil || !*ref.Controller {
			continue
		}
		if controller != nil {
			return metav1.OwnerReference{}, false
		}
		controller = ref
	}
	if controller != nil {
		return *controller, true
	}
	if len(refs) == 1 {
		return refs[0], true
	}
	return metav1.OwnerReference{}, false
}

func (p PodOwners) replicaSetGetter(
	namespace, name string,
) func() (metav1.Object, error) {
	if p.RS != nil {
		return func() (metav1.Object, error) {
			return p.RS.ReplicaSets(namespace).Get(name)
		}
	}
	if p.Client == nil {
		return nil
	}
	return func() (metav1.Object, error) {
		return p.Client.AppsV1().ReplicaSets(namespace).Get(
			p.callContext(), name, metav1.GetOptions{},
		)
	}
}

func (p PodOwners) daemonSetGetter(
	namespace, name string,
) func() (metav1.Object, error) {
	if p.DS != nil {
		return func() (metav1.Object, error) {
			return p.DS.DaemonSets(namespace).Get(name)
		}
	}
	if p.Client == nil {
		return nil
	}
	return func() (metav1.Object, error) {
		return p.Client.AppsV1().DaemonSets(namespace).Get(
			p.callContext(), name, metav1.GetOptions{},
		)
	}
}

func (p PodOwners) statefulSetGetter(
	namespace, name string,
) func() (metav1.Object, error) {
	if p.SS != nil {
		return func() (metav1.Object, error) {
			return p.SS.StatefulSets(namespace).Get(name)
		}
	}
	if p.Client == nil {
		return nil
	}
	return func() (metav1.Object, error) {
		return p.Client.AppsV1().StatefulSets(namespace).Get(
			p.callContext(), name, metav1.GetOptions{},
		)
	}
}

func (p PodOwners) callContext() context.Context {
	if p.Ctx != nil {
		return p.Ctx
	}
	return context.Background()
}

// OwnerFunc adapts a plain function to OwnerResolver, for a producer that is
// handed the pod pipeline's resolver rather than the listers.
type OwnerFunc func(pod *corev1.Pod) model.ObjectRef

// OwnerOf implements OwnerResolver.
func (f OwnerFunc) OwnerOf(pod *corev1.Pod) model.ObjectRef {
	if f == nil {
		return model.ObjectRef{}
	}
	return f(pod)
}
