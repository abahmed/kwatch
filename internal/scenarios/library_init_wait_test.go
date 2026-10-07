package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// initWaitScenarios are pods held in Init by a container that keeps
// running.
func initWaitScenarios() []scenario {
	return []scenario{
		initWaitsOnBrokenService(), initWaitsOnHealthyService(),
		initFinishesSlowly(),
	}
}

// initWaitsOnBrokenService: the api pod's wait-for-db init container has
// run for minutes because db has no ready endpoint: db's own pod
// crash-loops. The db Deployment is the cause; api is only waiting.
func initWaitsOnBrokenService() scenario {
	return scenario{
		expect: expectation{
			Name: "init-waits-on-broken-service",
			Description: "An init container waits for a Service whose " +
				"pods crash-loop, holding its pod in Init.",
			Root: "deployment/shop/db", Tier: "notify", MaxMessages: 3,
			MustNotBlame: []string{"deployment/shop/api"},
		},
		build: func(c *cluster) {
			db := initWaitDB(c)
			api := initWaitAPI(c)
			c.after(30 * time.Second)
			started := c.now
			for restarts := int32(1); restarts <= 6; restarts++ {
				c.update(db.pod(0, "n1", crashLoop(1, "Error",
					"FATAL: data directory has wrong ownership",
					restarts)))
				c.update(initWaitSlice(c, db, false))
				c.update(stuckPod(api, started))
				c.after(time.Minute)
			}
		},
	}
}

// initWaitsOnHealthyService: the same wait, but db is healthy. The pod is
// stuck in Init all the same; kwatch states the facts and blames api.
func initWaitsOnHealthyService() scenario {
	return scenario{
		expect: expectation{
			Name: "init-waits-on-healthy-service",
			Description: "An init container runs for minutes while the " +
				"Service it names is healthy.",
			Root: "deployment/shop/api", Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"deployment/shop/db"},
		},
		build: func(c *cluster) {
			initWaitDB(c)
			api := initWaitAPI(c)
			c.after(30 * time.Second)
			started := c.now
			for range 8 {
				c.update(stuckPod(api, started))
				c.after(time.Minute)
			}
		},
	}
}

// initFinishesSlowly: an init container runs for two minutes, then
// completes. That is under the limit and nothing to report.
func initFinishesSlowly() scenario {
	return scenario{
		expect: expectation{
			Name: "init-finishes-slowly",
			Description: "An init container runs for two minutes and " +
				"completes; the pod starts.",
			Quiet: true, Tail: duration(15 * time.Minute),
		},
		build: func(c *cluster) {
			initWaitDB(c)
			api := initWaitAPI(c)
			started := c.now
			c.update(stuckPod(api, started))
			c.after(2 * time.Minute)
			c.update(api.pod(0, "n1"))
		},
	}
}

// initWaitDB is the db Deployment with its Service; healthy at first.
func initWaitDB(c *cluster) *workload {
	c.list(c.node("n1", "zone-a"))
	db := c.deployment("shop", "db", "registry.example.com/db:15", 1)
	c.list(db.objects())
	c.list(db.pod(0, "n1"))
	c.list(clusterService(c, "shop", "db", 5432), initWaitSlice(c, db, true))
	return db
}

// initWaitSlice is db's EndpointSlice with its one pod ready or not.
func initWaitSlice(
	c *cluster, db *workload, ready bool,
) *discoveryv1.EndpointSlice {
	slice := clusterSlice(c, "shop", "db", db.pod(0, "n1"))
	slice.Endpoints[0].Conditions.Ready = boolPtr(ready)
	return slice
}

// initWaitAPI is the api Deployment whose pods wait for db in an init
// container; its pod is listed before the wait starts.
func initWaitAPI(c *cluster) *workload {
	api := c.deployment("shop", "api", "registry.example.com/api:4", 1)
	configTemplate(api, func(spec *corev1.PodSpec) {
		spec.InitContainers = []corev1.Container{{
			Name: "wait-for-db", Image: "registry.example.com/wait:1",
			Env: []corev1.EnvVar{{Name: "DB_ADDR", Value: "db:5432"}},
		}}
	})
	c.list(api.objects())
	return api
}

// initRunning is a pod whose init container started at started and is
// still running: the pod is Pending and its app container waits.
func initRunning(started time.Time) podState {
	return func(c *cluster, pod *corev1.Pod) {
		notReady(c, pod)
		pod.Status.Phase = corev1.PodPending
		init := pod.Spec.InitContainers[0]
		pod.Status.InitContainerStatuses = []corev1.ContainerStatus{{
			Name: init.Name, Image: init.Image, Started: boolPtr(true),
			State: corev1.ContainerState{
				Running: &corev1.ContainerStateRunning{
					StartedAt: metav1.NewTime(started)}},
		}}
		app := &pod.Status.ContainerStatuses[0]
		app.Started = boolPtr(false)
		app.State = corev1.ContainerState{
			Waiting: &corev1.ContainerStateWaiting{
				Reason: "PodInitializing"},
		}
	}
}

// stuckPod is the api pod created at started, its init container
// running since then.
func stuckPod(api *workload, started time.Time) *corev1.Pod {
	return api.pod(0, "n1", createdAt(started), initRunning(started))
}
