package app

import (
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

// Cluster shape of the memory budget test: 500 nodes, 1,000 workloads
// (800 Deployments with their ReplicaSets, 100 StatefulSets and 100
// DaemonSets) and 5,000 pods. Every Deployment has a Service, an
// EndpointSlice, a Secret and a ConfigMap its pods reference.
const (
	memNodes        = 500
	memNamespaces   = 50
	memDeployments  = 800
	memStatefulSets = 100
	memDaemonSets   = 100
	memDeployPods   = 5 // pods per Deployment: 4,000
	memSetPods      = 10
	// memFailingNode hosts the pods that crash, so the solve has a
	// shared cause to find.
	memFailingNode = 7
)

// memCluster holds the generated objects, grouped by schema.
type memCluster struct {
	namespaces   []any
	nodes        []any
	deployments  []any
	replicaSets  []any
	statefulSets []any
	daemonSets   []any
	pods         []any
	services     []any
	slices       []any
	secrets      []any
	configMaps   []any
}

func memMeta(ns, name string) metav1.ObjectMeta {
	return metav1.ObjectMeta{
		Name: name, Namespace: ns, UID: types.UID(ns + "/" + name),
		Labels: map[string]string{"app": name, "team": "shop"},
		Annotations: map[string]string{
			"deployment.kubernetes.io/revision": "3",
		},
		ResourceVersion: "1",
	}
}

func memOwner(kind, name string) []metav1.OwnerReference {
	yes := true
	return []metav1.OwnerReference{{
		Kind: kind, Name: name, Controller: &yes,
		UID: types.UID(kind + "/" + name),
	}}
}

// buildMemCluster generates the cluster. Pod i lands on node i mod
// memNodes, so every node runs ten pods.
func buildMemCluster(now time.Time) memCluster {
	var c memCluster
	for i := 0; i < memNamespaces; i++ {
		c.namespaces = append(c.namespaces, &corev1.Namespace{
			ObjectMeta: memMeta("", fmt.Sprintf("ns-%d", i)),
			Status: corev1.NamespaceStatus{
				Phase: corev1.NamespaceActive,
			},
		})
	}
	for i := 0; i < memNodes; i++ {
		c.nodes = append(c.nodes, memNode(i, now))
	}
	podIndex := 0
	for i := 0; i < memDeployments; i++ {
		ns := fmt.Sprintf("ns-%d", i%memNamespaces)
		name := fmt.Sprintf("web-%d", i)
		c.addDeployment(ns, name)
		for p := 0; p < memDeployPods; p++ {
			pod := memPod(ns, fmt.Sprintf("%s-7f-%d", name, p),
				"ReplicaSet", name+"-7f", name, podIndex, now)
			c.pods = append(c.pods, pod)
			podIndex++
		}
		c.addService(ns, name, memDeployPods)
	}
	for i := 0; i < memStatefulSets; i++ {
		ns := fmt.Sprintf("ns-%d", i%memNamespaces)
		name := fmt.Sprintf("db-%d", i)
		c.statefulSets = append(c.statefulSets, memStatefulSet(ns, name))
		for p := 0; p < memSetPods; p++ {
			c.pods = append(c.pods, memPod(ns, fmt.Sprintf("%s-%d",
				name, p), "StatefulSet", name, "", podIndex, now))
			podIndex++
		}
	}
	for i := 0; i < memDaemonSets; i++ {
		ns := fmt.Sprintf("ns-%d", i%memNamespaces)
		c.daemonSets = append(c.daemonSets,
			memDaemonSet(ns, fmt.Sprintf("agent-%d", i)))
	}
	return c
}

func memNode(i int, now time.Time) *corev1.Node {
	name := fmt.Sprintf("node-%d", i)
	meta := memMeta("", name)
	meta.Labels["topology.kubernetes.io/zone"] = fmt.Sprintf("z-%d", i%3)
	return &corev1.Node{
		ObjectMeta: meta,
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{{
				Type: corev1.NodeReady, Status: corev1.ConditionTrue,
				LastTransitionTime: metav1.NewTime(now.Add(-time.Hour)),
			}},
			NodeInfo: corev1.NodeSystemInfo{
				KubeletVersion: "v1.35.0", KernelVersion: "6.8.0",
				ContainerRuntimeVersion: "containerd://2.0.0",
				OperatingSystem:         "linux",
			},
		},
	}
}

// addDeployment adds a Deployment, its ReplicaSet, and the Secret and
// ConfigMap its pods read. Values are hashed like the informer
// transform does before the translator sees them.
func (c *memCluster) addDeployment(ns, name string) {
	replicas := int32(memDeployPods)
	c.deployments = append(c.deployments, &appsv1.Deployment{
		ObjectMeta: memMeta(ns, name),
		Spec: appsv1.DeploymentSpec{Replicas: &replicas,
			Template: memTemplate(name)},
		Status: appsv1.DeploymentStatus{Replicas: replicas,
			ReadyReplicas: replicas, AvailableReplicas: replicas,
			UpdatedReplicas: replicas, ObservedGeneration: 1},
	})
	rsMeta := memMeta(ns, name+"-7f")
	rsMeta.OwnerReferences = memOwner("Deployment", name)
	c.replicaSets = append(c.replicaSets, &appsv1.ReplicaSet{
		ObjectMeta: rsMeta,
		Spec: appsv1.ReplicaSetSpec{Replicas: &replicas,
			Template: memTemplate(name)},
		Status: appsv1.ReplicaSetStatus{Replicas: replicas,
			ReadyReplicas: replicas, AvailableReplicas: replicas},
	})
	c.secrets = append(c.secrets, &corev1.Secret{
		ObjectMeta: memMeta(ns, name+"-creds"),
		Data: map[string][]byte{
			"username": []byte("app"), "password": []byte("not-real"),
		},
	})
	c.configMaps = append(c.configMaps, &corev1.ConfigMap{
		ObjectMeta: memMeta(ns, name+"-config"),
		Data: map[string]string{
			"LOG_LEVEL": "info", "FEATURES": "a,b,c",
		},
	})
}

func (c *memCluster) addService(ns, name string, pods int) {
	c.services = append(c.services, &corev1.Service{
		ObjectMeta: memMeta(ns, name),
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app": name},
			Ports:    []corev1.ServicePort{{Name: "http", Port: 80}},
		},
	})
	meta := memMeta(ns, name+"-abc")
	meta.Labels[discoveryv1.LabelServiceName] = name
	slice := &discoveryv1.EndpointSlice{ObjectMeta: meta,
		AddressType: discoveryv1.AddressTypeIPv4}
	ready := true
	for p := 0; p < pods; p++ {
		slice.Endpoints = append(slice.Endpoints, discoveryv1.Endpoint{
			Addresses:  []string{fmt.Sprintf("10.0.%d.%d", p, p)},
			Conditions: discoveryv1.EndpointConditions{Ready: &ready},
			TargetRef: &corev1.ObjectReference{Kind: "Pod",
				Namespace: ns, Name: fmt.Sprintf("%s-7f-%d", name, p)},
		})
	}
	c.slices = append(c.slices, slice)
}

func memTemplate(name string) corev1.PodTemplateSpec {
	return corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{"app": name},
		},
		Spec: corev1.PodSpec{Containers: []corev1.Container{{
			Name: "app", Image: "registry.example/app:1",
		}}},
	}
}

func memStatefulSet(ns, name string) *appsv1.StatefulSet {
	replicas := int32(memSetPods)
	return &appsv1.StatefulSet{
		ObjectMeta: memMeta(ns, name),
		Spec: appsv1.StatefulSetSpec{Replicas: &replicas,
			Template: memTemplate(name)},
		Status: appsv1.StatefulSetStatus{Replicas: replicas,
			ReadyReplicas: replicas, AvailableReplicas: replicas,
			CurrentReplicas: replicas, UpdatedReplicas: replicas},
	}
}

func memDaemonSet(ns, name string) *appsv1.DaemonSet {
	return &appsv1.DaemonSet{
		ObjectMeta: memMeta(ns, name),
		Spec:       appsv1.DaemonSetSpec{Template: memTemplate(name)},
		Status: appsv1.DaemonSetStatus{
			DesiredNumberScheduled: memNodes,
			CurrentNumberScheduled: memNodes, NumberReady: memNodes,
			NumberAvailable: memNodes, UpdatedNumberScheduled: memNodes,
		},
	}
}

// memPod builds a running pod, or a crash-looping one on memFailingNode.
// Deployment pods (app != "") read their Secret and ConfigMap.
func memPod(
	ns, name, ownerKind, owner, app string, index int, now time.Time,
) *corev1.Pod {
	meta := memMeta(ns, name)
	meta.OwnerReferences = memOwner(ownerKind, owner)
	node := index % memNodes
	failing := node == memFailingNode
	container := corev1.Container{
		Name: "app", Image: "registry.example/app:1",
	}
	if app != "" {
		meta.Labels["app"] = app
		container.EnvFrom = []corev1.EnvFromSource{
			{SecretRef: &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: app + "-creds"}}},
			{ConfigMapRef: &corev1.ConfigMapEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: app + "-config"}}},
		}
	}
	return &corev1.Pod{
		ObjectMeta: meta,
		Spec: corev1.PodSpec{
			NodeName:   fmt.Sprintf("node-%d", node),
			Containers: []corev1.Container{container},
		},
		Status: memPodStatus(failing, now),
	}
}

func memPodStatus(failing bool, now time.Time) corev1.PodStatus {
	since := metav1.NewTime(now.Add(-30 * time.Minute))
	ready := corev1.ConditionTrue
	state := corev1.ContainerState{
		Running: &corev1.ContainerStateRunning{StartedAt: since},
	}
	restarts := int32(0)
	if failing {
		ready = corev1.ConditionFalse
		since = metav1.NewTime(now.Add(-10 * time.Minute))
		restarts = 6
		state = corev1.ContainerState{
			Waiting: &corev1.ContainerStateWaiting{
				Reason:  "CrashLoopBackOff",
				Message: "back-off 5m0s restarting failed container",
			},
		}
	}
	return corev1.PodStatus{
		Phase: corev1.PodRunning,
		Conditions: []corev1.PodCondition{{
			Type: corev1.PodReady, Status: ready,
			LastTransitionTime: since,
		}},
		ContainerStatuses: []corev1.ContainerStatus{{
			Name: "app", Ready: !failing, Image: "registry.example/app:1",
			RestartCount: restarts, State: state,
		}},
	}
}
