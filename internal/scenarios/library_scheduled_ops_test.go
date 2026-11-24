package scenarios

import (
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// A cluster that is put to sleep every evening and woken every morning
// by a schedule: at about 18:00 its apps are scaled to zero and its
// pool nodes are switched off and removed; at about 06:45 new nodes
// join and the apps are scaled up again. kwatch runs all the while on
// a node that stays.
const (
	// schedApps is how many apps the schedule scales.
	schedApps = 8
	// schedRouted is how many of them have a Service and an Ingress.
	schedRouted = 3
	// schedPool is how many pool nodes the schedule removes and adds.
	schedPool = 3
	// schedReadyDelay is how long the apps' probes fail in the morning.
	schedReadyDelay = 6 * time.Minute
)

// schedDay changes one day of the scheduled cluster. Zero values are
// the plain schedule.
type schedDay struct {
	// stayDown is the index of an app the morning scale-up leaves at
	// zero replicas; -1 for none.
	stayDown int
	// crash makes the crasher crash-loop with a cause of its own at
	// 10:00 on the day after this morning.
	crash bool
	// keepNode leaves the first pool node switched off but not removed
	// in this day's evening.
	keepNode bool
}

// schedCluster is the scheduled cluster over several days.
type schedCluster struct {
	c       *cluster
	apps    []*workload
	crasher *workload
	// pods are the live pods of each app, by app index.
	pods map[int]*corev1.Pod
	// pool are the live pool nodes.
	pool []*corev1.Node
	day  int
}

// schedStart is noon of the first day.
var schedStart = time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)

func newSchedCluster() *schedCluster {
	s := &schedCluster{c: newCluster(schedStart, ""),
		pods: map[int]*corev1.Pod{}}
	c := s.c
	c.list(c.node("core1", "zone-a"), c.node("core2", "zone-a"))
	s.pool = s.poolNodes(false)
	for i := range schedApps {
		w := c.deployment("shop", fmt.Sprintf("app%d", i),
			"registry.example.com/app:1.0", 1)
		s.apps = append(s.apps, w)
		pod := w.pod(0, s.pool[i%schedPool].Name)
		s.pods[i] = pod
		c.list(w.deployment, w.replicaSet, pod)
		if i < schedRouted {
			c.list(clusterService(c, "shop", w.deployment.Name, 8080))
			c.list(scaleDownIngress(c, w.deployment.Name))
		}
	}
	s.crasher = c.deployment("ops", "reporter",
		"registry.example.com/reporter:1.0", 1)
	c.list(s.crasher.deployment, s.crasher.replicaSet,
		s.crasher.pod(0, "core1"))
	return s
}

// poolNodes builds the pool nodes of the current day; created marks
// them as joining now rather than listed.
func (s *schedCluster) poolNodes(created bool) []*corev1.Node {
	c := s.c
	nodes := make([]*corev1.Node, 0, schedPool)
	for k := range schedPool {
		n := c.node(fmt.Sprintf("pool-d%d-%d", s.day, k), "zone-a")
		if created {
			n.CreationTimestamp = metav1.NewTime(c.now)
			c.create(n)
			c.after(20 * time.Second)
		} else {
			c.list(n)
		}
		nodes = append(nodes, n)
	}
	return nodes
}

// at moves the clock to hh:mm of the current day.
func (s *schedCluster) at(hour, minute int) {
	day := schedStart.AddDate(0, 0, s.day)
	target := time.Date(day.Year(), day.Month(), day.Day(), hour, minute,
		0, 0, time.UTC)
	s.c.after(target.Sub(s.c.now))
}

// evening scales the apps to zero, then switches the pool nodes off:
// they stop reporting and are removed a few minutes later.
func (s *schedCluster) evening(d schedDay) {
	c := s.c
	s.at(18, 0)
	for i, w := range s.apps {
		zero := int32(0)
		w.deployment.Spec.Replicas = &zero
		w.replicaSet.Spec.Replicas = &zero
		w.setReady(0)
		c.update(w.objects())
		if pod, ok := s.pods[i]; ok {
			c.remove(last(c, pod))
			delete(s.pods, i)
		}
		c.after(10 * time.Second)
	}
	c.after(2 * time.Minute)
	for _, n := range s.pool {
		nodeLose(c, n)
	}
	c.after(3 * time.Minute)
	for i, n := range s.pool {
		if i > 0 || !d.keepNode {
			c.remove(last(c, n))
		}
	}
	s.pool = nil
}

// morning adds new pool nodes and scales the apps up again; their pods
// fail the readiness probe for schedReadyDelay, then pass.
func (s *schedCluster) morning(d schedDay) {
	c := s.c
	s.day++
	s.at(6, 45)
	s.pool = s.poolNodes(true)
	for i, w := range s.apps {
		if i == d.stayDown {
			continue
		}
		one := int32(1)
		w.deployment.Spec.Replicas = &one
		w.replicaSet.Spec.Replicas = &one
		w.setReady(0)
		c.update(w.objects())
		pod := w.pod(s.day, s.pool[i%schedPool].Name, startedNow,
			notReady)
		c.create(pod)
		s.pods[i] = pod
		c.after(10 * time.Second)
	}
	c.after(2 * time.Minute)
	for i := range s.apps {
		if pod, ok := s.pods[i]; ok {
			c.warn(c.warningEvent(pod, "Pod", "Unhealthy",
				"Readiness probe failed: HTTP probe failed with "+
					"statuscode: 503", "kubelet", 3))
		}
	}
	c.after(schedReadyDelay)
	for i, w := range s.apps {
		pod, ok := s.pods[i]
		if !ok {
			continue
		}
		ready := w.pod(s.day, pod.Spec.NodeName,
			createdAt(pod.CreationTimestamp.Time))
		c.update(ready)
		s.pods[i] = ready
		w.setReady(1)
		c.update(w.objects())
	}
}

// crashOwnCause makes the crasher crash-loop for a reason of its own.
func (s *schedCluster) crashOwnCause() {
	c, w := s.c, s.crasher
	s.at(10, 0)
	for _, restarts := range []int32{2, 4, 6} {
		c.update(w.pod(0, "core1", crashLoop(1, "Error",
			"panic: invalid configuration: missing key REPORT_BUCKET",
			restarts)))
		w.setReady(0)
		c.update(w.objects())
		c.after(2 * time.Minute)
	}
}

// schedDays builds days of the schedule: day 0 is the first evening,
// each next day a morning, the following evening, and the given
// changes.
func schedDays(days []schedDay) *schedCluster {
	s := newSchedCluster()
	s.evening(schedDay{})
	for _, d := range days {
		s.morning(d)
		if d.crash {
			s.crashOwnCause()
		}
		s.evening(d)
	}
	return s
}

// restartAt starts a new session at hh:mm of the next day: the new
// cluster lists every live object, as informers do after a restart.
func (s *schedCluster) restartAt(hour, minute int) {
	day := schedStart.AddDate(0, 0, s.day+1)
	at := time.Date(day.Year(), day.Month(), day.Day(), hour, minute, 0,
		0, time.UTC)
	c := newCluster(at, "")
	c.list(remaining(s.c)...)
	for _, w := range append([]*workload{s.crasher}, s.apps...) {
		w.c = c
	}
	s.c = c
}
