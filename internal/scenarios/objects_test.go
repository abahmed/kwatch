package scenarios

import (
	"crypto/sha256"
	"encoding/hex"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// meta builds object metadata with the instance name and namespace. The
// UID is derived from the key so it is stable across regenerations.
func (c *cluster) meta(namespace, name string) metav1.ObjectMeta {
	ns := ""
	if namespace != "" {
		ns = c.n(namespace)
	}
	sum := sha256.Sum256([]byte(ns + "/" + c.n(name)))
	return metav1.ObjectMeta{
		Name: c.n(name), Namespace: ns,
		UID:               types.UID(hex.EncodeToString(sum[:8])),
		CreationTimestamp: metav1.NewTime(c.start.Add(-24 * time.Hour)),
	}
}

// node builds a Ready node in zone.
func (c *cluster) node(name, zone string) *corev1.Node {
	meta := c.meta("", name)
	meta.Labels = map[string]string{
		corev1.LabelHostname: meta.Name,
	}
	if zone != "" {
		meta.Labels[corev1.LabelTopologyZone] = c.n(zone)
	}
	return &corev1.Node{
		ObjectMeta: meta,
		Status: corev1.NodeStatus{
			Allocatable: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("4"),
				corev1.ResourceMemory: resource.MustParse("16Gi"),
			},
			Conditions: []corev1.NodeCondition{{
				Type: corev1.NodeReady, Status: corev1.ConditionTrue,
				Reason:             "KubeletReady",
				LastTransitionTime: metav1.NewTime(c.start.Add(-time.Hour)),
			}},
		},
	}
}

// setNodeCondition sets or adds one node condition as the kubelet or the
// node lifecycle controller would.
func setNodeCondition(
	node *corev1.Node, kind corev1.NodeConditionType,
	status corev1.ConditionStatus, reason, message string, since time.Time,
) {
	for i := range node.Status.Conditions {
		if node.Status.Conditions[i].Type == kind {
			node.Status.Conditions[i] = corev1.NodeCondition{
				Type: kind, Status: status, Reason: reason,
				Message: message, LastTransitionTime: metav1.NewTime(since),
			}
			return
		}
	}
	node.Status.Conditions = append(node.Status.Conditions,
		corev1.NodeCondition{
			Type: kind, Status: status, Reason: reason, Message: message,
			LastTransitionTime: metav1.NewTime(since),
		})
}

// workload is a Deployment, its current ReplicaSet and the pod template
// they share.
type workload struct {
	c          *cluster
	deployment *appsv1.Deployment
	replicaSet *appsv1.ReplicaSet
	// revision counts rollouts; it names the ReplicaSet.
	revision int
}

// deployment builds a healthy Deployment and its ReplicaSet.
func (c *cluster) deployment(
	namespace, name, image string, replicas int32,
) *workload {
	meta := c.meta(namespace, name)
	labels := map[string]string{"app": meta.Name}
	d := &appsv1.Deployment{
		ObjectMeta: meta,
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: podTemplate(labels, image),
		},
	}
	d.Generation, d.Status.ObservedGeneration = 1, 1
	w := &workload{c: c, deployment: d, revision: 1}
	w.replicaSet = w.newReplicaSet()
	w.setReady(replicas)
	return w
}

func podTemplate(
	labels map[string]string, image string,
) corev1.PodTemplateSpec {
	return corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{Labels: labels},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{
			Name: "app", Image: image,
		}}},
	}
}

func (w *workload) newReplicaSet() *appsv1.ReplicaSet {
	d := w.deployment
	sum := sha256.Sum256([]byte(d.Name + d.Spec.Template.String()))
	name := d.Name + "-" + hex.EncodeToString(sum[:4])
	yes := true
	rs := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: d.Namespace,
			UID: types.UID(name), Labels: d.Spec.Template.Labels,
			CreationTimestamp: metav1.NewTime(w.c.now),
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "apps/v1", Kind: "Deployment", Name: d.Name,
				UID: d.UID, Controller: &yes,
			}},
		},
		Spec: appsv1.ReplicaSetSpec{
			Replicas: d.Spec.Replicas, Template: d.Spec.Template,
		},
	}
	rs.Status.Replicas = *d.Spec.Replicas
	rs.Status.ReadyReplicas = *d.Spec.Replicas
	rs.Status.AvailableReplicas = *d.Spec.Replicas
	return rs
}

// objects returns the Deployment and ReplicaSet for listing.
func (w *workload) objects() (*appsv1.Deployment, *appsv1.ReplicaSet) {
	return w.deployment, w.replicaSet
}

// setReady sets the Deployment and ReplicaSet status to ready replicas.
func (w *workload) setReady(ready int32) {
	d, rs := w.deployment, w.replicaSet
	want := *d.Spec.Replicas
	d.Status.Replicas, d.Status.UpdatedReplicas = want, want
	d.Status.ReadyReplicas, d.Status.AvailableReplicas = ready, ready
	d.Status.UnavailableReplicas = want - ready
	rs.Status.Replicas = want
	rs.Status.ReadyReplicas, rs.Status.AvailableReplicas = ready, ready
}

// rollout changes the pod template, as kubectl set image does, and
// starts a new ReplicaSet. The caller delivers both objects.
func (w *workload) rollout(mutate func(*corev1.PodSpec)) *appsv1.ReplicaSet {
	d := w.deployment.DeepCopy()
	mutate(&d.Spec.Template.Spec)
	d.Generation++
	d.Status.ObservedGeneration = d.Generation
	w.deployment = d
	w.revision++
	w.replicaSet = w.newReplicaSet()
	w.replicaSet.Status = appsv1.ReplicaSetStatus{}
	return w.replicaSet
}

// podState edits a pod into one runtime state.
type podState func(c *cluster, pod *corev1.Pod)

// pod builds replica i of the workload's current ReplicaSet on node, in
// the running and ready state unless states say otherwise.
func (w *workload) pod(i int, node string, states ...podState) *corev1.Pod {
	rs := w.replicaSet
	yes := true
	name := rs.Name + "-" + string(rune('a'+i%26)) + itoa(i/26)
	started := metav1.NewTime(w.c.start.Add(-time.Hour))
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: name, Namespace: rs.Namespace, UID: types.UID(name),
			Labels:            rs.Spec.Template.Labels,
			CreationTimestamp: started,
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "apps/v1", Kind: "ReplicaSet", Name: rs.Name,
				UID: rs.UID, Controller: &yes,
			}},
		},
		Spec: *rs.Spec.Template.Spec.DeepCopy(),
	}
	if node != "" {
		pod.Spec.NodeName = w.c.n(node)
	}
	running(w.c, pod)
	for _, state := range states {
		state(w.c, pod)
	}
	return pod
}

func itoa(i int) string {
	if i == 0 {
		return ""
	}
	digits := ""
	for ; i > 0; i /= 10 {
		digits = string(rune('0'+i%10)) + digits
	}
	return digits
}

// running is a started, ready pod.
func running(c *cluster, pod *corev1.Pod) {
	since := metav1.NewTime(c.start.Add(-time.Hour))
	pod.Status = corev1.PodStatus{
		Phase: corev1.PodRunning, StartTime: &since,
		QOSClass: corev1.PodQOSBurstable,
		Conditions: []corev1.PodCondition{
			{Type: corev1.PodScheduled, Status: corev1.ConditionTrue,
				LastTransitionTime: since},
			{Type: corev1.PodReady, Status: corev1.ConditionTrue,
				LastTransitionTime: since},
			{Type: corev1.ContainersReady, Status: corev1.ConditionTrue,
				LastTransitionTime: since},
		},
	}
	for _, container := range pod.Spec.Containers {
		pod.Status.ContainerStatuses = append(pod.Status.ContainerStatuses,
			corev1.ContainerStatus{
				Name: container.Name, Image: container.Image, Ready: true,
				Started: boolPtr(true),
				State: corev1.ContainerState{
					Running: &corev1.ContainerStateRunning{StartedAt: since},
				},
			})
	}
}

// startedNow marks the pod as started at the current time, as a fresh
// replica of a rollout is.
func startedNow(c *cluster, pod *corev1.Pod) {
	now := metav1.NewTime(c.now)
	pod.CreationTimestamp = now
	pod.Status.StartTime = &now
	for i := range pod.Status.Conditions {
		pod.Status.Conditions[i].LastTransitionTime = now
	}
	for i := range pod.Status.ContainerStatuses {
		if r := pod.Status.ContainerStatuses[i].State.Running; r != nil {
			r.StartedAt = now
		}
	}
}

// createdAt dates the pod's creation and start at, as a replica a
// rollout created then is, while later states move the clock on.
func createdAt(at time.Time) podState {
	return func(_ *cluster, pod *corev1.Pod) {
		created := metav1.NewTime(at)
		pod.CreationTimestamp = created
		pod.Status.StartTime = &created
	}
}

// notReady turns the pod's readiness off since the current time.
func notReady(c *cluster, pod *corev1.Pod) {
	now := metav1.NewTime(c.now)
	for i := range pod.Status.Conditions {
		switch pod.Status.Conditions[i].Type {
		case corev1.PodReady, corev1.ContainersReady:
			pod.Status.Conditions[i].Status = corev1.ConditionFalse
			pod.Status.Conditions[i].Reason = "ContainersNotReady"
			pod.Status.Conditions[i].LastTransitionTime = now
		}
	}
	for i := range pod.Status.ContainerStatuses {
		pod.Status.ContainerStatuses[i].Ready = false
	}
}

// crashLoop is a container that keeps exiting with code and message.
func crashLoop(
	exitCode int32, reason, message string, restarts int32,
) podState {
	return func(c *cluster, pod *corev1.Pod) {
		notReady(c, pod)
		status := &pod.Status.ContainerStatuses[0]
		status.Started = boolPtr(false)
		status.RestartCount = restarts
		status.State = corev1.ContainerState{
			Waiting: &corev1.ContainerStateWaiting{
				Reason:  "CrashLoopBackOff",
				Message: "back-off 5m0s restarting failed container",
			},
		}
		status.LastTerminationState = corev1.ContainerState{
			Terminated: &corev1.ContainerStateTerminated{
				ExitCode: exitCode, Reason: reason, Message: message,
				StartedAt:  metav1.NewTime(c.now.Add(-20 * time.Second)),
				FinishedAt: metav1.NewTime(c.now.Add(-10 * time.Second)),
			},
		}
	}
}

// waiting is a container that cannot start, such as ImagePullBackOff or
// CreateContainerConfigError.
func waiting(reason, message string) podState {
	return func(c *cluster, pod *corev1.Pod) {
		notReady(c, pod)
		status := &pod.Status.ContainerStatuses[0]
		status.Started = boolPtr(false)
		status.State = corev1.ContainerState{
			Waiting: &corev1.ContainerStateWaiting{
				Reason: reason, Message: message,
			},
		}
	}
}

// pendingUnscheduled is a pod the scheduler cannot place.
func pendingUnscheduled(message string) podState {
	return func(c *cluster, pod *corev1.Pod) {
		now := metav1.NewTime(c.now)
		pod.Spec.NodeName = ""
		pod.CreationTimestamp = now
		pod.Status = corev1.PodStatus{
			Phase: corev1.PodPending, QOSClass: corev1.PodQOSBurstable,
			Conditions: []corev1.PodCondition{{
				Type: corev1.PodScheduled, Status: corev1.ConditionFalse,
				Reason: "Unschedulable", Message: message,
				LastTransitionTime: now,
			}},
		}
	}
}

// evicted is a pod the kubelet evicted.
func evicted(message string) podState {
	return func(c *cluster, pod *corev1.Pod) {
		notReady(c, pod)
		pod.Status.Phase = corev1.PodFailed
		pod.Status.Reason = "Evicted"
		pod.Status.Message = message
		for i := range pod.Status.ContainerStatuses {
			pod.Status.ContainerStatuses[i].State = corev1.ContainerState{
				Terminated: &corev1.ContainerStateTerminated{
					ExitCode: 137, Reason: "ContainerStatusUnknown",
					FinishedAt: metav1.NewTime(c.now),
				},
			}
		}
	}
}

// warningEvent builds a Warning event about obj, as a controller or the
// kubelet reports it. kind is the Kubernetes Kind of obj.
func (c *cluster) warningEvent(
	obj metav1.Object, kind, reason, message, source string, count int32,
) *corev1.Event {
	at := metav1.NewTime(c.now)
	// Typed fixtures leave TypeMeta empty and resolve to the built-in
	// group; custom resources carry their apiVersion.
	apiVersion := ""
	if typed, ok := obj.(interface{ GetAPIVersion() string }); ok {
		apiVersion = typed.GetAPIVersion()
	}
	return &corev1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name: obj.GetName() + "." + reason, Namespace: obj.GetNamespace(),
		},
		InvolvedObject: corev1.ObjectReference{
			APIVersion: apiVersion, Kind: kind,
			Namespace: obj.GetNamespace(), Name: obj.GetName(),
		},
		Type: corev1.EventTypeWarning, Reason: reason, Message: message,
		Source: corev1.EventSource{Component: source}, Count: count,
		FirstTimestamp: at, LastTimestamp: at,
	}
}

func boolPtr(v bool) *bool { return &v }
