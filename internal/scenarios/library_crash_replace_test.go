package scenarios

import (
	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// crashReplaceScenarios are crash loops that last while Karpenter keeps
// replacing the nodes under them.
func crashReplaceScenarios() []scenario {
	return []scenario{crashLoopDuringNodeReplacement()}
}

// fileNotFoundLine is the log line of the replaced-assembly crash.
const fileNotFoundLine = "FileNotFoundException: Could not load file or " +
	"assembly 'Trella.Models.Common, Version=3.1.120.0'"

// crashLoopDuringNodeReplacement: both replicas of a Deployment abort
// (exit 134, empty termination message) on every start for an hour.
// The pods carry a completed monitoring init container. Karpenter
// replaces a node of the two-node pool every eight minutes, so a pool
// is always booting, and each replacement restarts a pod on the new
// node. The Deployment's HPA cannot read metrics. The crash is the
// failure; the node churn and the autoscaler are not.
func crashLoopDuringNodeReplacement() scenario {
	return scenario{
		expect: expectation{
			Name: "crash-loop-during-node-replacement",
			Description: "Both replicas of a Deployment abort on start " +
				"for an hour while Karpenter keeps replacing nodes and " +
				"the HPA cannot read metrics.",
			Root: "deployment/data/warehouse", Tier: "notify",
			MaxMessages: 6,
			MustNotBlame: []string{"nodepool//" + bootPool,
				"horizontalpodautoscaler/data/warehouse"},
			Tail: duration(10 * time.Minute),
		},
		build: buildCrashDuringReplacement,
	}
}

// poolNode is a node of the boot pool, created at born.
func poolNode(c *cluster, name string, born time.Time) *corev1.Node {
	node := c.node(name, "zone-a")
	node.Labels["karpenter.sh/nodepool"] = bootPool
	node.CreationTimestamp = metav1.NewTime(born)
	return node
}

// withMonitoringInit adds the completed datadog init container.
func withMonitoringInit(c *cluster, pod *corev1.Pod) {
	init := pod.Spec.InitContainers[0]
	pod.Status.InitContainerStatuses = []corev1.ContainerStatus{{
		Name: init.Name, Image: init.Image, Ready: true,
		State: corev1.ContainerState{Terminated: &corev1.
			ContainerStateTerminated{Reason: "Completed",
			FinishedAt: metav1.NewTime(c.start)}},
	}}
}

func buildCrashDuringReplacement(c *cluster) {
	old := c.start.Add(-2 * time.Minute)
	nodes := []string{"n1", "n2"}
	c.list(poolNode(c, "n1", old), poolNode(c, "n2", old))
	w := c.deployment("data", "warehouse",
		"registry.example.com/warehouse:3.1", 2)
	configTemplate(w, func(spec *corev1.PodSpec) {
		spec.InitContainers = []corev1.Container{{
			Name: "datadog-init", Image: "registry.example.com/dd:1"}}
	})
	c.list(w.objects())
	index := []int{0, 1}
	born := []time.Time{c.start, c.start}
	for i := range index {
		c.list(w.pod(index[i], nodes[i], func(c *cluster, p *corev1.Pod) {
			withMonitoringInit(c, p)
		}))
	}
	c.list(clusterHPA(c, "data", "warehouse", "True", "ValidMetricFound", ""))
	others := bystanders(c)
	c.after(time.Minute)
	for range 3 {
		morningCrash(c, w, nodes, index, born)
	}
	for i := range born {
		born[i] = c.now
	}
	next := 3
	for minute := 0; minute < 60; minute++ {
		if minute%8 == 7 {
			i := (minute / 8) % 2
			c.remove(w.pod(index[i], nodes[i]),
				poolNode(c, nodes[i], old))
			nodes[i] = "n" + strconv.Itoa(next)
			next++
			c.create(poolNode(c, nodes[i], c.now))
			index[i] += 2
			born[i] = c.now
		}
		crashStep(c, w, nodes, index, born)
		bystandersLoseMetrics(c, others)
		c.after(time.Minute)
	}
}

// crashStep shows both pods aborting once more, the Deployment with
// nothing ready and the HPA unable to read their metrics.
func crashStep(
	c *cluster, w *workload, nodes []string, index []int, born []time.Time,
) {
	for i := range index {
		restarts := int32(c.now.Sub(born[i]) / (2 * time.Minute))
		pod := w.pod(index[i], nodes[i], crashLoop(134, "Error", "",
			restarts+1), func(c *cluster, p *corev1.Pod) {
			withMonitoringInit(c, p)
		})
		c.update(pod)
		id := kube.ContainerID(pod.Namespace, pod.Name, "app")
		c.emit(kube.CrashLogObservation(id, c.now,
			[]string{"info: starting", fileNotFoundLine}))
	}
	w.setReady(0)
	c.update(w.objects())
	hpa := clusterHPA(c, "data", "warehouse", "False",
		"FailedGetResourceMetric", metricsServerFailure)
	c.update(hpa)
	c.warn(c.warningEvent(hpa, "HorizontalPodAutoscaler",
		"FailedGetResourceMetric", metricsServerFailure,
		"horizontal-pod-autoscaler", 1))
}

// bystanders lists healthy Deployments, each with an HPA, on the pool.
func bystanders(c *cluster) []string {
	names := []string{"shop", "billing", "search", "orders", "users",
		"media"}
	for _, ns := range names {
		w := c.deployment(ns, "web", "registry.example.com/web:7", 2)
		c.list(w.objects())
		c.list(w.pod(0, "n1"), w.pod(1, "n2"))
		c.list(clusterHPA(c, ns, "web", "True", "ValidMetricFound", ""))
	}
	return names
}

// bystandersLoseMetrics makes every bystander HPA fail to read its metrics.
func bystandersLoseMetrics(c *cluster, namespaces []string) {
	for _, ns := range namespaces {
		hpa := clusterHPA(c, ns, "web", "False",
			"FailedGetResourceMetric", metricsServerFailure)
		c.update(hpa)
		c.warn(c.warningEvent(hpa, "HorizontalPodAutoscaler",
			"FailedGetResourceMetric", metricsServerFailure,
			"horizontal-pod-autoscaler", 1))
	}
}

// morningCrash is a startup crash of six minutes that ends by itself:
// the daily noise of a workload that comes up with the cluster.
func morningCrash(
	c *cluster, w *workload, nodes []string, index []int, born []time.Time,
) {
	for range 6 {
		crashStep(c, w, nodes, index, born)
		c.after(time.Minute)
	}
	for i := range index {
		c.update(w.pod(index[i], nodes[i], startedNow))
	}
	w.setReady(2)
	c.update(w.objects())
	c.update(clusterHPA(c, "data", "warehouse", "True",
		"ValidMetricFound", ""))
	c.after(23*time.Hour + 54*time.Minute)
}
