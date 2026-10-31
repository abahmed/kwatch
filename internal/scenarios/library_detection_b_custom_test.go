package scenarios

import (
	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func detectionBCustomScenarios() []scenario {
	return []scenario{
		customTaskFailed(), customControllerFailedEvents(),
		customHealthyWarningStream(), customWarningsThenRecovery(),
	}
}

// taskRun is a pipeline task custom resource whose controller reports
// its outcome as a Succeeded condition.
func taskRun(c *cluster, name string) *unstructured.Unstructured {
	meta := clusterMeta(c, "ci", name, "taskrun")
	u := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "tekton.example.com/v1", "kind": "TaskRun",
		"spec":   map[string]any{},
		"status": map[string]any{"observedGeneration": int64(1)},
	}}
	u.SetName(meta.Name)
	u.SetNamespace(meta.Namespace)
	u.SetUID(meta.UID)
	u.SetGeneration(1)
	return u
}

// customTaskFailed: a task run's controller reports Succeeded=False with
// the step that failed. The condition is bad news by its own status, and
// the controller's message is quoted.
func customTaskFailed() scenario {
	return scenario{
		expect: expectation{
			Name: "custom-task-succeeded-false",
			Description: "A custom resource reports Succeeded=False " +
				"with a message: the failure is reported and the " +
				"message is quoted.",
			Root: "taskrun.tekton.example.com/ci/build-image", Tier: "notify",
			MaxMessages: 2, Tail: duration(30 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			run := taskRun(c, "build-image")
			clusterCondition(run, "Succeeded", "False", "Failed",
				"step build failed: exit status 2", c.now)
			c.list(run)
			c.after(10 * time.Minute)
		},
	}
}

// customControllerFailedEvents: a custom resource's controller reports a
// Warning event with the reason "Failed" again and again. That reason is
// a pod's state elsewhere, not this resource's: it is reported as an
// unusual event with the controller's words.
func customControllerFailedEvents() scenario {
	return scenario{
		expect: expectation{
			Name: "custom-controller-failed-events",
			Description: "A custom resource's controller repeats a " +
				"Warning event reasoned Failed: a digest line that " +
				"quotes it.",
			Root: "order.acme.example.com/infra/web-tls", Tier: "digest",
			MaxMessages: 2, Tail: duration(time.Hour),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			order := acmeOrder(c)
			c.list(order)
			for n := range 12 {
				c.warn(c.warningEvent(order, "Order", "Failed",
					"Failed to finalize Order: 429 too many requests",
					"acme-controller", int32(n+1)))
				c.after(4 * time.Minute)
			}
		},
	}
}

// singleWarning is one fresh Warning event with a count of one and no
// series, as a controller writes about each item it processes.
func singleWarning(
	c *cluster, order *unstructured.Unstructured, n int,
) *corev1.Event {
	event := c.warningEvent(order, "Order", "ItemRejected",
		"a processed item was rejected", "acme-controller", 1)
	event.Name += "." + strconv.Itoa(n)
	if event.Namespace == "" {
		event.Namespace = "default"
	}
	return event
}

// customHealthyWarningStream: a controller keeps writing Warnings about
// what it processes onto its own cluster-scoped resource, which stays
// Ready=True. They say nothing about the resource's health.
func customHealthyWarningStream() scenario {
	return scenario{
		expect: expectation{
			Name: "custom-healthy-warning-stream",
			Description: "A cluster-scoped Ready=True custom resource " +
				"receives single Warning events past the event window: " +
				"nothing is reported.",
			Quiet: true, Tail: duration(30 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			order := acmeOrder(c)
			order.SetNamespace("")
			clusterCondition(order, "Ready", "True", "Reconciled", "ok",
				c.now.Add(-time.Hour))
			c.list(order)
			for n := range 16 {
				c.warn(singleWarning(c, order, n))
				c.after(2 * time.Minute)
			}
		},
	}
}

// customWarningsThenRecovery: Warnings on a resource with no status open
// a digest incident. It turns Ready=True while the Warnings go on, and
// the incident resolves.
func customWarningsThenRecovery() scenario {
	return scenario{
		expect: expectation{
			Name: "custom-warnings-then-recovery",
			Description: "A custom resource's Warnings open an incident; " +
				"once it is Ready=True the incident resolves although " +
				"Warnings continue.",
			Root: "order.acme.example.com/infra/web-tls", Tier: "digest",
			MaxMessages: 2, Tail: duration(10 * time.Minute),
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			order := acmeOrder(c)
			c.list(order)
			warn := func(n int) {
				c.warn(c.warningEvent(order, "Order", "ItemRejected",
					"a processed item was rejected", "acme-controller",
					int32(n+1)))
				c.after(2 * time.Minute)
			}
			for n := range 16 {
				warn(n)
			}
			healthy := order.DeepCopy()
			clusterCondition(healthy, "Ready", "True", "Reconciled", "ok",
				c.now)
			c.update(healthy)
			for n := 16; n < 36; n++ {
				warn(n)
			}
		},
	}
}

// acmeOrder is a healthy certificate order of a certificate controller.
func acmeOrder(c *cluster) *unstructured.Unstructured {
	meta := clusterMeta(c, "infra", "web-tls", "order")
	u := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "acme.example.com/v1", "kind": "Order",
		"spec":   map[string]any{},
		"status": map[string]any{"observedGeneration": int64(1)},
	}}
	u.SetName(meta.Name)
	u.SetNamespace(meta.Namespace)
	u.SetUID(meta.UID)
	u.SetGeneration(1)
	return u
}
