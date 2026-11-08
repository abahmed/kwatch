package kube

import (
	"context"
	"sort"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/safego"
)

const (
	logSampleInterval = 30 * time.Second
	// maxSampleReads bounds the API calls of one round, so sampling is
	// rate-limited whatever the cluster size.
	maxSampleReads = 6
	// sampleDeniedRounds is how many rounds a refused read holds every
	// sample back: a missing permission costs one request per 5 minutes.
	sampleDeniedRounds = 10
)

// SampleLogReader reads the recent output of a running container.
type SampleLogReader interface {
	SampleLines(
		ctx context.Context, container inventory.EntityID,
		since time.Duration,
	) ([]string, error)
}

// LogSampleConfig configures the log sample round.
type LogSampleConfig struct {
	Logs   SampleLogReader
	Model  inventory.Reader
	Now    func() time.Time
	Submit Submit
}

// LogSampleRound samples the log of one Running and Ready pod of each
// workload every logSampleWindow and records how many error lines the
// window held, against the windows before it. It exists for the failure
// Kubernetes cannot see: pods that pass every platform check while the
// application reports errors. It reads at most maxSampleReads logs a
// round, one pod per workload, and a refused read stops it quietly.
type LogSampleRound struct {
	cfg LogSampleConfig
	// history holds, per workload, its recent windows, oldest first.
	history map[inventory.EntityID][]windowCount
	// rounds counts the passes made; deniedUntil is the round that may
	// read again after a refusal. start rotates the workloads.
	rounds, deniedUntil, start int
}

// NewLogSampleRound builds the round.
func NewLogSampleRound(cfg LogSampleConfig) *LogSampleRound {
	return &LogSampleRound{cfg: cfg,
		history: map[inventory.EntityID][]windowCount{}}
}

// Run samples every logSampleInterval until ctx ends.
func (r *LogSampleRound) Run(ctx context.Context) {
	ticker := time.NewTicker(logSampleInterval)
	defer ticker.Stop()
	for {
		safego.Do("log-sample-round", func() { r.Round(ctx) })
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// Round makes one pass over the workloads that are due.
func (r *LogSampleRound) Round(ctx context.Context) {
	r.rounds++
	workloads := r.workloads()
	r.forgetGone(workloads)
	if r.rounds < r.deniedUntil || len(workloads) == 0 {
		return
	}
	var observations []inventory.Observation
	reads, next := 0, 0
	for i := range workloads {
		idx := (r.start + i) % len(workloads)
		container, ok := r.due(workloads[idx])
		if !ok {
			continue
		}
		if reads == maxSampleReads || ctx.Err() != nil {
			next = idx
			break
		}
		reads++
		observation, stop := r.sample(ctx, workloads[idx], container)
		if observation != nil {
			observations = append(observations, *observation)
		}
		if stop {
			break
		}
	}
	r.start = next
	if len(observations) > 0 {
		r.cfg.Submit(ctx, observations...)
	}
}

// due returns the container to read for a workload that is due and has
// a pod Running and Ready.
func (r *LogSampleRound) due(
	w inventory.EntityID,
) (inventory.EntityID, bool) {
	windows := r.history[w]
	if len(windows) > 0 &&
		r.cfg.Now().Before(windows[len(windows)-1].at.Add(logSampleWindow)) {
		return inventory.EntityID{}, false
	}
	return r.sampleTarget(w)
}

// sample reads one container and describes the window. stop is set by
// a refusal, which holds every read back.
func (r *LogSampleRound) sample(
	ctx context.Context, w, container inventory.EntityID,
) (observation *inventory.Observation, stop bool) {
	lines, err := r.cfg.Logs.SampleLines(ctx, container, logSampleWindow)
	if err != nil {
		return nil, r.refused(err)
	}
	o := r.record(w, lines)
	return &o, false
}

// workloads lists the workloads sampled, sorted.
func (r *LogSampleRound) workloads() []inventory.EntityID {
	var out []inventory.EntityID
	for _, kind := range []inventory.Kind{
		KindDeployment, KindStatefulSet, KindDaemonSet} {
		out = append(out, r.cfg.Model.Entities(kind)...)
	}
	return out
}

// sampleTarget picks the container to read: the first container of a
// pod of w that is Running and Ready, rotating over the pods.
func (r *LogSampleRound) sampleTarget(
	w inventory.EntityID,
) (inventory.EntityID, bool) {
	var ready []inventory.EntityID
	for _, pod := range r.cfg.Model.Related(w, inventory.OwnedBy,
		inventory.Incoming) {
		if pod.Kind == KindPod && r.runningReady(pod) {
			ready = append(ready, pod)
		}
	}
	if len(ready) == 0 {
		return inventory.EntityID{}, false
	}
	sort.Slice(ready, func(a, b int) bool {
		return ready[a].Name < ready[b].Name
	})
	pod := ready[len(r.history[w])%len(ready)]
	for _, c := range r.cfg.Model.Entities(KindContainer) {
		if c.Namespace == pod.Namespace &&
			strings.HasPrefix(c.Name, pod.Name+"/") {
			return c, true
		}
	}
	return inventory.EntityID{}, false
}

func (r *LogSampleRound) runningReady(pod inventory.EntityID) bool {
	e, ok := r.cfg.Model.Entity(pod)
	if !ok {
		return false
	}
	phase, _ := e.Attribute(AttrPhase)
	ready, _ := e.Attribute(AttrReady)
	isReady, _ := ready.Value.AsBool()
	return phase.Value.AsText() == "Running" && isReady
}

// record adds a window to the workload's history and describes it.
func (r *LogSampleRound) record(
	w inventory.EntityID, lines []string,
) inventory.Observation {
	now := r.cfg.Now()
	last := countErrors(lines, now)
	var earlier []windowCount
	for _, old := range r.history[w] {
		if now.Sub(old.at) <= logSampleHistory {
			earlier = append(earlier, old)
		}
	}
	r.history[w] = append(earlier, last)
	return sampleObservation(w, now, last, earlier)
}

// refused notes a failed read and reports a refusal, which holds every
// read back. It is a permission gap, not an incident.
func (r *LogSampleRound) refused(err error) bool {
	if !apierrors.IsForbidden(err) {
		return false
	}
	r.deniedUntil = r.rounds + sampleDeniedRounds
	klog.V(1).InfoS("log sampling paused: pods/log not permitted",
		"component", "inventory", "operation", "log-sample")
	return true
}

// forgetGone drops the history of workloads the model no longer has.
func (r *LogSampleRound) forgetGone(live []inventory.EntityID) {
	if len(r.history) == 0 {
		return
	}
	keep := make(map[inventory.EntityID]bool, len(live))
	for _, id := range live {
		keep[id] = true
	}
	for id := range r.history {
		if !keep[id] {
			delete(r.history, id)
		}
	}
}
