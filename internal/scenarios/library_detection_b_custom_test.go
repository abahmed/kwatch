package scenarios

import (
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func detectionBCustomScenarios() []scenario {
	return []scenario{customTaskFailed(), customControllerFailedEvents()}
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
