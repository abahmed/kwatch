package scenarios

import (
	"fmt"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/replay"
)

// The soak day is a long, busy day of a mid-size cluster: about ninety
// pods in thirty Deployments on a few nodes, a rollout every fifteen
// minutes, scale events, nodes that come and go, a Warning event every
// few seconds, kubelet usage samples and three Deployments that crash
// for hours. It exists to see whether memory plateaus: the same shapes
// repeat all day, so a structure that keeps growing is a leak.
const (
	soakWorkloads   = 30
	soakNodes       = 4
	soakEventEvery  = 20 * time.Second
	soakStatsEvery  = 10 * time.Minute
	soakNodeEvery   = 2 * time.Hour
	soakNodeLife    = 50 * time.Minute
	soakCrashers    = 3
	soakCrashPeriod = 5 * time.Minute
)

var soakReasons = []string{
	"BackOff", "Unhealthy", "FailedMount", "FailedScheduling", "Evicted",
	"FailedCreate", "ProbeWarning", "NodeNotReady", "ImagePullBackOff",
}

// soakStep is one thing that happens at a simulated time.
type soakStep struct {
	at  time.Time
	run func()
}

// soakLog builds the soak day of the given length.
func soakLog(length time.Duration) replay.Log {
	c := newCluster(stagingStart, "")
	nodes := make([]string, soakNodes)
	for i := range nodes {
		nodes[i] = fmt.Sprintf("soak-%d", i)
		c.list(c.node(nodes[i], backgroundPrefix+"zone"))
	}
	workloads := make([]*workload, soakWorkloads)
	for i := range workloads {
		w := c.deployment(backgroundNamespace, fmt.Sprintf("app-%d", i),
			fmt.Sprintf("registry.example.com/app-%d:1", i), 3)
		c.list(w.objects())
		for r := range 3 {
			c.list(w.pod(r, nodes[(i+r)%len(nodes)]))
		}
		workloads[i] = w
	}
	var steps []soakStep
	add := func(at time.Duration, run func()) {
		steps = append(steps, soakStep{stagingStart.Add(at), run})
	}
	soakRollouts(c, workloads, nodes, length, add)
	soakScaling(c, workloads, nodes, length, add)
	soakNodeChurn(c, length, add)
	soakEvents(c, workloads, nodes, length, add)
	soakStats(c, workloads, nodes, length, add)
	soakCrashes(c, workloads, nodes, length, add)
	sort.SliceStable(steps, func(i, j int) bool {
		return steps[i].at.Before(steps[j].at)
	})
	for _, step := range steps {
		c.now = step.at
		step.run()
	}
	return c.log()
}

// soakRollouts rolls one of the first half of the Deployments every
// fifteen minutes.
func soakRollouts(
	c *cluster, ws []*workload, nodes []string, length time.Duration,
	add func(time.Duration, func()),
) {
	for at, i := rolloutEvery, 0; at < length; at, i = at+rolloutEvery,
		i+1 {
		w := ws[i%(soakWorkloads/2)]
		add(at, func() { successfulRollout(c, w, nodes) })
	}
}

// soakScaling scales the second half out and back.
func soakScaling(
	c *cluster, ws []*workload, nodes []string, length time.Duration,
	add func(time.Duration, func()),
) {
	half := soakWorkloads / 2
	for at, i := scaleEvery, 0; at < length; at, i = at+scaleEvery, i+1 {
		w := ws[half+i%(soakWorkloads-half-soakCrashers)]
		add(at, func() { scaleOut(c, w, nodes) })
		add(at+scaleHold, func() { scaleBack(c, w) })
	}
}

// soakNodeChurn adds two new nodes every two hours and removes them later.
func soakNodeChurn(
	c *cluster, length time.Duration, add func(time.Duration, func()),
) {
	for at, i := soakNodeEvery, 0; at < length; at, i = at+soakNodeEvery,
		i+1 {
		var made []runtime.Object
		for k := range 2 {
			name := fmt.Sprintf("extra-%d-%d", i, k)
			add(at, func() {
				n := c.node(name, backgroundPrefix+"zone")
				made = append(made, n)
				c.create(n)
			})
		}
		add(at+soakNodeLife, func() { c.remove(made...) })
	}
}

// soakEvents is a Warning event about some live pod every few seconds.
func soakEvents(
	c *cluster, ws []*workload, nodes []string, length time.Duration,
	add func(time.Duration, func()),
) {
	n := 0
	for at := soakEventEvery; at < length; at += soakEventEvery {
		n++
		i := n
		add(at, func() {
			w := ws[i%soakWorkloads]
			pod := w.pod(i%3, nodes[i%len(nodes)])
			reason := soakReasons[i%len(soakReasons)]
			c.warn(c.warningEvent(pod, "Pod", reason,
				fmt.Sprintf("%s attempt %d", reason, i%97), "kubelet",
				int32(i%50+1)))
		})
	}
}

// soakStats is the kubelet's usage samples of every pod.
func soakStats(
	c *cluster, ws []*workload, nodes []string, length time.Duration,
	add func(time.Duration, func()),
) {
	for at := soakStatsEvery; at < length; at += soakStatsEvery {
		i := int64(at / soakStatsEvery)
		add(at, func() {
			for k, w := range ws {
				for r := range 3 {
					pod := w.pod(r, nodes[(k+r)%len(nodes)])
					c.emit(inventory.Observation{
						Kind: inventory.Observed, Source: kube.StatsSource,
						At: c.now, Entity: podID(pod),
						Attributes: map[string]inventory.Value{
							kube.AttrMemoryWorking: inventory.Number(
								float64(100<<20 + (i*int64(k+1))%(50<<20))),
						},
					})
				}
			}
		})
	}
}

// soakCrashes keeps the last Deployments crash-looping all day.
func soakCrashes(
	c *cluster, ws []*workload, nodes []string, length time.Duration,
	add func(time.Duration, func()),
) {
	for k := range soakCrashers {
		w := ws[soakWorkloads-1-k]
		restarts := int32(1)
		for at := time.Minute; at < length; at += soakCrashPeriod {
			add(at+time.Duration(k)*time.Second, func() {
				restarts++
				pod := w.pod(0, nodes[k%len(nodes)], crashLoop(1, "Error",
					"panic: boom", restarts))
				c.update(pod)
			})
		}
	}
}

func podID(pod *corev1.Pod) inventory.EntityID {
	return inventory.EntityID{
		Kind: kube.KindFor("Pod"), Namespace: pod.Namespace, Name: pod.Name,
	}
}
