package scenarios

import (
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// livenessCascadeScenarios are restarts that a failing dependency
// causes through a liveness probe that checks the dependency.
func livenessCascadeScenarios() []scenario {
	return []scenario{livenessCascade(), livenessOwnFault()}
}

// livenessCascade: the database loses its disk and its only pod
// crash-loops, so Service db has no ready endpoints. The API and the
// worker call it, and their liveness probes run the same /healthz
// check as readiness, which reads the database. The kubelet restarts
// every pod, but the restarts are a symptom. The database is the root
// and the restarts join its incident.
func livenessCascade() scenario {
	return scenario{
		expect: expectation{
			Name: "liveness-cascade",
			Description: "A database fails; the API and worker pods are " +
				"restarted by a liveness probe that is the same check " +
				"as readiness.",
			Root: "deployment/shop/db", Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"deployment/shop/api",
				"deployment/shop/worker", "node//n1"},
		},
		build: buildLivenessCascade,
	}
}

// livenessOwnFault: the same workloads and probes, but the database is
// healthy. The API is killed by its own liveness probe, so nothing
// outside it is blamed.
func livenessOwnFault() scenario {
	return scenario{
		expect: expectation{
			Name: "liveness-cascade-healthy-dependency",
			Description: "API pods are killed by a liveness probe that " +
				"is the same check as readiness while the database " +
				"they call is healthy.",
			Root: "deployment/shop/api", Tier: "notify", MaxMessages: 2,
			MustNotBlame: []string{"deployment/shop/db", "node//n1"},
		},
		build: func(c *cluster) {
			c.list(c.node("n1", "zone-a"))
			db := c.deployment("shop", "db", "registry.example.com/db:15", 1)
			c.list(db.objects())
			dbPod := db.pod(0, "n1")
			c.list(dbPod, clusterService(c, "shop", "db", 5432),
				trafficSlice(c, "db", dbPod))
			api := cascadeApp(c, "api", 3)
			c.after(time.Minute)
			cascadeKills(c, api, 3)
		},
	}
}

func buildLivenessCascade(c *cluster) {
	c.list(c.node("n1", "zone-a"))
	db := c.deployment("shop", "db", "registry.example.com/db:15", 1)
	c.list(db.objects())
	dbPod := db.pod(0, "n1")
	c.list(dbPod, clusterService(c, "shop", "db", 5432),
		trafficSlice(c, "db", dbPod))
	api := cascadeApp(c, "api", 3)
	worker := cascadeApp(c, "worker", 2)
	c.after(time.Minute)
	for restarts := int32(1); restarts <= 6; restarts++ {
		c.update(db.pod(0, "n1", crashLoop(1, "Error",
			"FATAL: could not write to file: No space left on device",
			restarts)))
		db.setReady(0)
		c.update(db.objects())
		c.update(trafficUnreadySlice(c, "db", db.pod(0, "n1")))
		c.after(20 * time.Second)
		cascadeKillRound(c, api, 3, restarts+2)
		cascadeKillRound(c, worker, 2, restarts+2)
		api.setReady(0)
		worker.setReady(0)
		c.update(api.objects())
		c.update(worker.objects())
		c.after(40 * time.Second)
	}
}

// cascadeApp lists a workload whose containers read the database
// address from DB_HOST and whose liveness and readiness probes both
// run GET /healthz on port 8080.
func cascadeApp(c *cluster, name string, replicas int32) *workload {
	w := c.deployment("shop", name, "registry.example.com/"+name+":5",
		replicas)
	check := corev1.ProbeHandler{HTTPGet: &corev1.HTTPGetAction{
		Path: "/healthz", Port: intstr.FromInt32(8080)}}
	configTemplate(w, func(spec *corev1.PodSpec) {
		spec.Containers[0].Env = []corev1.EnvVar{{
			Name: "DB_HOST", Value: c.n("db") + ":5432"}}
		spec.Containers[0].ReadinessProbe = &corev1.Probe{
			ProbeHandler: check}
		spec.Containers[0].LivenessProbe = &corev1.Probe{
			ProbeHandler: check, PeriodSeconds: 10, FailureThreshold: 3}
	})
	c.list(w.objects())
	for i := range int(replicas) {
		c.list(w.pod(i, "n1"))
	}
	return w
}

// cascadeKills restarts the pods of w five times, 45 seconds apart.
func cascadeKills(c *cluster, w *workload, replicas int) {
	for restarts := int32(3); restarts <= 7; restarts++ {
		cascadeKillRound(c, w, replicas, restarts)
		c.after(45 * time.Second)
	}
}

// cascadeKillRound is one liveness kill of every pod of w.
func cascadeKillRound(c *cluster, w *workload, replicas int, restarts int32) {
	probe := "Liveness probe failed: HTTP probe failed with statuscode: 503"
	for i := range replicas {
		pod := w.pod(i, "n1", liveKilled(restarts, 40*time.Second))
		ev := c.warningEvent(pod, "Pod", "Unhealthy", probe, "kubelet",
			restarts*3)
		ev.InvolvedObject.FieldPath = "spec.containers{app}"
		c.warn(ev)
		c.update(pod)
	}
}
