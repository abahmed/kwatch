package scenarios

import (
	"fmt"
	"hash/fnv"
	"sort"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/abahmed/kwatch/internal/audit"
	"github.com/abahmed/kwatch/internal/replay"
	"github.com/abahmed/kwatch/internal/scorecard"
)

// The staging day is a busy staging cluster: every labelled scenario
// happens once, at a spread-out time, and is fixed
// stagingFix later, while background workloads roll out and scale
// successfully the whole day and the explicit non-events of
// nonevent_test.go (rollouts, scaling, a drain, Jobs, a CronJob) run
// twice each.
const (
	stagingSpacing    = 9 * time.Minute
	stagingFix        = 45 * time.Minute
	stagingBackground = 24
	// Healthy churn: one successful rollout every rolloutEvery and one
	// scale event every scaleEvery, across the background workloads.
	rolloutEvery = 15 * time.Minute
	scaleEvery   = 25 * time.Minute
)

// stagingDay is the length of the staging day. It grows with the scenario
// library, one stagingSpacing per scenario (twelve hours for the 79
// scenarios the gate was set on), so the alert density the peak gate
// measures does not depend on how many scenarios the library holds.
func stagingDay() time.Duration {
	return stagingSpacing * time.Duration(len(stagingScenarios())+1)
}

// multiDay is how long a scenario may last and still be part of the
// staging day. A problem that is known needs two earlier days of history,
// so a scenario of days has no place in a day.
const multiDay = 24 * time.Hour

// stagingScenarios are the scenarios that happen in the staging day: the
// library, without the scenarios that last more than a day.
func stagingScenarios() []scenario {
	var out []scenario
	for _, s := range library() {
		log := s.generate(stagingStart, "")
		if n := len(log.Entries); n > 0 &&
			log.Entries[n-1].At.Sub(log.Start) > multiDay {
			continue
		}
		out = append(out, s)
	}
	return out
}

var stagingStart = time.Date(2026, 9, 30, 7, 0, 0, 0, time.UTC)

type stagingResult struct {
	messages    int
	window      time.Duration
	peakPerHour int
	entries     []audit.Entry
	// nonEvents counts the messages attributable to a non-event, and
	// loudNonEvents those of them that notify or page.
	nonEvents, loudNonEvents int
}

func (s stagingResult) meanPerHour() float64 {
	if s.window <= 0 {
		return 0
	}
	return float64(s.messages) / s.window.Hours()
}

func runStagingDay(t *testing.T) stagingResult {
	t.Helper()
	log := stagingLog()
	result := replayLog(t, log, replay.Options{Tail: time.Hour})
	all, loud := nonEventNotifications(result.Decisions)
	return stagingResult{
		messages: len(result.Messages), window: result.End.Sub(log.Start),
		peakPerHour: scorecard.PeakInWindow(result.Times, time.Hour),
		entries:     auditEntries("staging", result),
		nonEvents:   all, loudNonEvents: loud,
	}
}

// stagingLog interleaves one instance of every scenario with the
// background churn and the non-events.
func stagingLog() replay.Log {
	scenarios := stagingScenarios()
	logs := append([]replay.Log{backgroundChurn()}, nonEventLogs()...)
	spacing := stagingSpacing
	for i, s := range scenarios {
		start := stagingStart.Add(spacing*time.Duration(i+1) +
			jitter(s.expect.Name, spacing/3))
		logs = append(logs, stagingInstance(s, start, i))
	}
	return merge(stagingStart, logs...)
}

// stagingInstance records one scenario with its own names, then the fix:
// everything it created is deleted and probed services recover.
func stagingInstance(s scenario, start time.Time, i int) replay.Log {
	c := newCluster(start, fmt.Sprintf("-s%d", i))
	s.build(c)
	c.after(stagingFix)
	for _, id := range c.probed {
		c.probe(id, "")
	}
	c.remove(remaining(c)...)
	return c.log()
}

// remaining lists the cluster's live objects, pods first so owners go
// last, as a namespace deletion removes them.
func remaining(c *cluster) []runtime.Object {
	keys := make([]string, 0, len(c.objects))
	for key := range c.objects {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		pi, pj := isPod(c.objects[keys[i]]), isPod(c.objects[keys[j]])
		if pi != pj {
			return pi
		}
		return keys[i] < keys[j]
	})
	out := make([]runtime.Object, 0, len(keys))
	for _, key := range keys {
		out = append(out, c.objects[key])
	}
	return out
}

func isPod(obj runtime.Object) bool {
	_, ok := obj.(*corev1.Pod)
	return ok
}

// jitter is a stable offset in [0, spread) derived from a name, so the
// day is the same every run.
func jitter(name string, spread time.Duration) time.Duration {
	h := fnv.New64a()
	_, _ = h.Write([]byte(name))
	return time.Duration(h.Sum64() % uint64(spread))
}

// backgroundChurn is the healthy part of the day: rollouts that succeed
// and scale events, none of which may alert.
func backgroundChurn() replay.Log {
	c := newCluster(stagingStart, "")
	nodes := []string{"bg-1", "bg-2", "bg-3", "bg-4"}
	for _, name := range nodes {
		c.list(c.node(name, backgroundPrefix+"zone"))
	}
	workloads := make([]*workload, 0, stagingBackground)
	for i := range stagingBackground {
		w := c.deployment(backgroundNamespace, fmt.Sprintf("web-%d", i),
			fmt.Sprintf("registry.example.com/web-%d:1", i), 3)
		c.list(w.objects())
		for r := range 3 {
			c.list(w.pod(r, nodes[(i+r)%len(nodes)]))
		}
		workloads = append(workloads, w)
	}
	for _, e := range stagingEvents() {
		c.now = e.at
		w := workloads[e.index]
		switch e.kind {
		case churnRollout:
			successfulRollout(c, w, nodes)
		case churnScaleOut:
			scaleOut(c, w, nodes)
		case churnScaleBack:
			scaleBack(c, w)
		}
	}
	return c.log()
}

type churnKind uint8

const (
	churnRollout churnKind = iota
	churnScaleOut
	churnScaleBack
)

// churnEvent is one background change. Rollouts use the first half of
// the background workloads and scale events the second, so no workload
// is scaled while it rolls out.
type churnEvent struct {
	at    time.Time
	index int
	kind  churnKind
}

func stagingEvents() []churnEvent {
	half := stagingBackground / 2
	var events []churnEvent
	day := stagingDay()
	for at, i := rolloutEvery, 0; at < day; at, i = at+rolloutEvery,
		i+1 {
		events = append(events, churnEvent{
			at: stagingStart.Add(at), index: i % half, kind: churnRollout,
		})
	}
	for at, i := scaleEvery, 0; at < day; at, i = at+scaleEvery,
		i+1 {
		out := stagingStart.Add(at + 7*time.Minute)
		index := half + i%half
		events = append(events,
			churnEvent{at: out, index: index, kind: churnScaleOut},
			churnEvent{at: out.Add(scaleHold), index: index,
				kind: churnScaleBack})
	}
	sort.SliceStable(events, func(i, j int) bool {
		return events[i].at.Before(events[j].at)
	})
	return events
}

// scaleHold is how long a scaled-out workload keeps its extra replicas.
const scaleHold = 20 * time.Minute

// successfulRollout replaces every replica with one of a new image: new
// pods start, become ready within half a minute, then old pods go.
func successfulRollout(c *cluster, w *workload, nodes []string) {
	old := make([]runtime.Object, 0, 3)
	for r := range 3 {
		old = append(old, last(c, w.pod(r, "")))
	}
	oldRS := w.replicaSet
	rs := w.rollout(func(spec *corev1.PodSpec) {
		spec.Containers[0].Image += "1"
	})
	c.update(w.deployment)
	c.create(rs)
	for r := range 3 {
		c.create(w.pod(r, nodes[r%len(nodes)], startedNow, notReady))
	}
	c.after(25 * time.Second)
	for r := range 3 {
		c.update(w.pod(r, nodes[r%len(nodes)], startedNow))
	}
	w.setReady(3)
	c.update(w.deployment, w.replicaSet)
	c.after(5 * time.Second)
	c.remove(old...)
	c.remove(oldRS)
}

// scaleOut scales a workload from three to five replicas; the new pods
// become ready within twenty seconds.
func scaleOut(c *cluster, w *workload, nodes []string) {
	setReplicas(w, 5)
	c.update(w.deployment, w.replicaSet)
	for r := 3; r < 5; r++ {
		c.create(w.pod(r, nodes[r%len(nodes)], startedNow, notReady))
	}
	c.after(20 * time.Second)
	for r := 3; r < 5; r++ {
		c.update(w.pod(r, nodes[r%len(nodes)], startedNow))
	}
	w.setReady(5)
	c.update(w.deployment, w.replicaSet)
}

// scaleBack returns a scaled-out workload to three replicas.
func scaleBack(c *cluster, w *workload) {
	extra := []runtime.Object{last(c, w.pod(3, "")), last(c, w.pod(4, ""))}
	setReplicas(w, 3)
	w.setReady(3)
	c.update(w.deployment, w.replicaSet)
	c.remove(extra...)
}

func setReplicas(w *workload, replicas int32) {
	d := w.deployment.DeepCopy()
	d.Spec.Replicas = &replicas
	d.Generation++
	d.Status.ObservedGeneration = d.Generation
	w.deployment = d
	rs := w.replicaSet.DeepCopy()
	rs.Spec.Replicas = &replicas
	w.replicaSet = rs
}
