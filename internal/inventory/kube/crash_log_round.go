package kube

import (
	"context"
	"sort"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/inventory"
)

const (
	// AttrLastErrorLine is the first error line of a crashed
	// container's previous log, for containers whose termination
	// message is empty. It is quoted, never interpreted.
	AttrLastErrorLine = "last.error.line"

	crashLogSource   = "crash-log"
	crashLogInterval = 30 * time.Second
	// maxCrashLogReads bounds the API calls of one round.
	maxCrashLogReads = 20
	crashLoopReason  = "CrashLoopBackOff"
	// errorBackoffRounds is how many rounds a container whose read
	// failed (not "no previous log") is left alone before a retry.
	errorBackoffRounds = 3
)

// PreviousLogReader reads the output of a container's previous run.
type PreviousLogReader interface {
	PreviousLines(
		ctx context.Context, container inventory.EntityID,
	) ([]string, error)
}

// CrashLogConfig configures the crash-log round.
type CrashLogConfig struct {
	Logs   PreviousLogReader
	Model  inventory.Reader
	Now    func() time.Time
	Submit Submit
}

// crashRun identifies one termination of a container: a new restart
// count, or a new finish time under the same count (a pod re-created
// under the same name), is a new run to read.
type crashRun struct {
	restarts int
	finished time.Time
}

// CrashLogRound reads, for crashed containers, the first error line of
// the previous run's log, once per run. Containers killed by a probe
// leave nothing in their termination message, so this line is the only
// shared cause the logs hold.
type CrashLogRound struct {
	cfg CrashLogConfig
	// read remembers the runs already read. It only holds containers
	// the model still has. Only the Run goroutine touches it.
	read map[inventory.EntityID]crashRun
	// rounds counts the passes made; retryAt holds, per container whose
	// read failed, the round that may try it again.
	rounds  int
	retryAt map[inventory.EntityID]int
	// start is where the next pass begins in the sorted containers, so
	// a budget that runs out reaches every container over time.
	start  int
	health selfHealthClock
	// denied is whether the last read was refused (no pods/log access).
	denied bool
}

// NewCrashLogRound builds the round.
func NewCrashLogRound(cfg CrashLogConfig) *CrashLogRound {
	return &CrashLogRound{cfg: cfg,
		read:    map[inventory.EntityID]crashRun{},
		retryAt: map[inventory.EntityID]int{}}
}

// Run reads crash logs every crashLogInterval until ctx ends.
func (r *CrashLogRound) Run(ctx context.Context) {
	ticker := time.NewTicker(crashLogInterval)
	defer ticker.Stop()
	for {
		r.Round(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// Round makes one pass over the model's containers.
func (r *CrashLogRound) Round(ctx context.Context) {
	containers := r.cfg.Model.Entities(KindContainer)
	sort.Slice(containers, func(i, j int) bool {
		return containers[i].String() < containers[j].String()
	})
	r.forgetGone(containers)
	r.rounds++
	r.logSelfHealth(r.cfg.Now())
	var observations []inventory.Observation
	reads := 0
	next := 0
	for i := range containers {
		idx := (r.start + i) % len(containers)
		id := containers[idx]
		run, crashed := r.crashedRun(id)
		if !crashed {
			observations = append(observations, r.forgetRun(id)...)
			continue
		}
		if r.read[id] == run || r.retryAt[id] > r.rounds {
			continue
		}
		if reads == maxCrashLogReads || ctx.Err() != nil {
			next = idx
			break
		}
		reads++
		lines, err := r.cfg.Logs.PreviousLines(ctx, id)
		if err != nil {
			r.noteError(err)
			if apierrors.IsForbidden(err) {
				break
			}
			observations = append(observations,
				r.afterError(id, run, err)...)
			continue
		}
		r.denied = false
		r.read[id] = run
		observations = append(observations,
			CrashLogObservation(id, r.cfg.Now(), lines))
	}
	r.start = next
	if len(observations) > 0 {
		r.cfg.Submit(ctx, observations...)
	}
}

// afterError decides what a failed read means. "No previous log" (not
// found, or a bad request for a container that has not terminated) will
// not change for this run, so the run counts as read and the line of an
// earlier run is removed. Anything else may be transient: the container
// waits a few rounds, so a few failing containers cannot use up every
// round's budget.
func (r *CrashLogRound) afterError(
	id inventory.EntityID, run crashRun, err error,
) []inventory.Observation {
	if apierrors.IsNotFound(err) || apierrors.IsBadRequest(err) {
		r.read[id] = run
		return []inventory.Observation{
			CrashLogObservation(id, r.cfg.Now(), nil)}
	}
	r.retryAt[id] = r.rounds + errorBackoffRounds
	return nil
}

// forgetRun removes the error line a container got while it was crashing,
// once it is not crashed any more (its last exit was clean).
func (r *CrashLogRound) forgetRun(
	id inventory.EntityID,
) []inventory.Observation {
	if _, had := r.read[id]; !had {
		return nil
	}
	delete(r.read, id)
	return []inventory.Observation{
		CrashLogObservation(id, r.cfg.Now(), nil)}
}

// crashedRun reports the container's last termination when it
// restarted after a failure: crash looping, or a non-zero last exit.
func (r *CrashLogRound) crashedRun(id inventory.EntityID) (crashRun, bool) {
	e, ok := r.cfg.Model.Entity(id)
	if !ok {
		return crashRun{}, false
	}
	restarts, _ := e.Attribute(AttrRestarts)
	count, _ := restarts.Value.AsNumber()
	if count < 1 {
		return crashRun{}, false
	}
	state, _ := e.Attribute(AttrStateReason)
	code, hasCode := e.Attribute(AttrLastExitCode)
	exit, _ := code.Value.AsNumber()
	if state.Value.AsText() != crashLoopReason && (!hasCode || exit == 0) {
		return crashRun{}, false
	}
	finished, _ := e.Attribute(AttrLastFinished)
	return crashRun{restarts: int(count),
		finished: finished.Value.AsTime()}, true
}

// CrashLogObservation is what the round records for a container whose
// previous run printed lines: the first error line, when there is one.
func CrashLogObservation(
	id inventory.EntityID, at time.Time, lines []string,
) inventory.Observation {
	attrs := map[string]inventory.Value{}
	if line := FirstErrorLine(lines); line != "" {
		attrs[AttrLastErrorLine] = inventory.Text(line)
	}
	return inventory.Observation{
		Kind: inventory.Observed, Source: crashLogSource,
		At: at, Entity: id, Attributes: attrs,
	}
}

// forgetGone drops the state of containers the model no longer has.
func (r *CrashLogRound) forgetGone(live []inventory.EntityID) {
	if len(r.read) == 0 && len(r.retryAt) == 0 {
		return
	}
	keep := make(map[inventory.EntityID]bool, len(live))
	for _, id := range live {
		keep[id] = true
	}
	for id := range r.read {
		if !keep[id] {
			delete(r.read, id)
		}
	}
	for id := range r.retryAt {
		if !keep[id] {
			delete(r.retryAt, id)
		}
	}
}

// noteError logs a refused or failed read once per streak.
func (r *CrashLogRound) noteError(err error) {
	if r.denied {
		return
	}
	r.denied = apierrors.IsForbidden(err)
	klog.V(1).InfoS("crash log unavailable", "component", "crash-log",
		"forbidden", r.denied, "error", err)
}
