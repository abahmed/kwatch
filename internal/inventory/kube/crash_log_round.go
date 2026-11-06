package kube

import (
	"context"
	"sync/atomic"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/safego"
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

// CrashLogConfig configures the crash-log round. Current is optional:
// without it the round reads no running container's log.
type CrashLogConfig struct {
	Logs    PreviousLogReader
	Current CurrentLogReader
	Model   inventory.Reader
	Now     func() time.Time
	Submit  Submit
}

// crashRun identifies one termination of a container: a new restart
// count, or a new finish time under the same count (a pod re-created
// under the same name), is a new run to read.
type crashRun struct {
	restarts int
	finished time.Time
	// short is a run that stopped within shortRun of its start.
	short bool
}

// shortRun is how long a run may last and still count as stopping at
// start: long enough for a slow start a liveness probe gives up on.
const shortRun = 5 * time.Minute

// StartLogReader also reads a short run's start (LogReader does).
type StartLogReader interface {
	PreviousLinesFromStart(
		ctx context.Context, container inventory.EntityID,
	) ([]string, error)
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
	// probes holds, per running container whose probe gets an HTTP
	// error status, when its current log was read (probe_log_round.go).
	probes map[inventory.EntityID]probeRead
	// start is where the next pass begins in the sorted containers, so
	// a budget that runs out reaches every container over time.
	start  int
	health selfHealthClock
	// denied is whether the last read was refused (no pods/log access).
	// It is read by other goroutines through PodLogsForbidden.
	denied atomic.Bool
	// hold is the backoff after a refused read (see crash_log_denied.go).
	hold deniedHold
}

// NewCrashLogRound builds the round.
func NewCrashLogRound(cfg CrashLogConfig) *CrashLogRound {
	return &CrashLogRound{cfg: cfg,
		read:    map[inventory.EntityID]crashRun{},
		retryAt: map[inventory.EntityID]int{},
		probes:  map[inventory.EntityID]probeRead{}}
}

// Run reads crash logs every crashLogInterval until ctx ends.
func (r *CrashLogRound) Run(ctx context.Context) {
	ticker := time.NewTicker(crashLogInterval)
	defer ticker.Stop()
	for {
		// A panicking round loses that round only; the next one runs.
		safego.Do("crash-log-round", func() { r.Round(ctx) })
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// Round makes one pass over the model's containers: the crashed ones
// first, then the running ones whose probe gets an HTTP error status.
// Both share the round's read budget and the backoff after a refusal.
func (r *CrashLogRound) Round(ctx context.Context) {
	// Entities is sorted, so the rotation (r.start) visits the same
	// containers in the same order every round.
	containers := r.cfg.Model.Entities(KindContainer)
	r.forgetGone(containers)
	r.rounds++
	r.logSelfHealth(r.cfg.Now())
	pass := readPass{holding: r.holding(containers)}
	observations := r.crashPass(ctx, containers, &pass)
	observations = append(observations,
		r.probePass(ctx, containers, &pass)...)
	if len(observations) > 0 {
		r.cfg.Submit(ctx, observations...)
	}
}

// readPass is the state one round's passes share: whether reads are
// held back after a refusal, and how many reads were made.
type readPass struct {
	holding bool
	reads   int
}

// spent reports a round that may make no more reads.
func (p *readPass) spent(ctx context.Context) bool {
	return p.holding || p.reads == maxCrashLogReads || ctx.Err() != nil
}

// crashPass reads the previous run of the crashed containers.
func (r *CrashLogRound) crashPass(
	ctx context.Context, containers []inventory.EntityID, pass *readPass,
) []inventory.Observation {
	var observations []inventory.Observation
	next := 0
	for i := range containers {
		idx := (r.start + i) % len(containers)
		id := containers[idx]
		run, crashed := r.crashedRun(id)
		if !crashed {
			observations = append(observations, r.forgetRun(id)...)
			continue
		}
		if pass.holding || r.read[id] == run || r.retryAt[id] > r.rounds {
			continue
		}
		if pass.spent(ctx) {
			next = idx
			break
		}
		pass.reads++
		lines, err := r.previous(ctx, id, run)
		if err != nil {
			if r.refused(err, containers, pass) {
				break
			}
			observations = append(observations,
				r.afterError(id, run, err)...)
			continue
		}
		r.denied.Store(false)
		r.read[id] = run
		observations = append(observations,
			CrashLogObservation(id, r.cfg.Now(), lines))
	}
	r.start = next
	return observations
}

// previous reads a run's log: from its start too when the run was short
// and the reader can (StartLogReader), else its tail only.
func (r *CrashLogRound) previous(
	ctx context.Context, id inventory.EntityID, run crashRun,
) ([]string, error) {
	if s, ok := r.cfg.Logs.(StartLogReader); ok && run.short {
		return s.PreviousLinesFromStart(ctx, id)
	}
	return r.cfg.Logs.PreviousLines(ctx, id)
}

// refused notes a failed read and reports a refusal, which holds every
// further read back (see crash_log_denied.go).
func (r *CrashLogRound) refused(
	err error, containers []inventory.EntityID, pass *readPass,
) bool {
	r.noteError(err)
	if !apierrors.IsForbidden(err) {
		return false
	}
	r.holdAfterDenial(containers)
	pass.holding = true
	return true
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
	started, _ := e.Attribute(AttrLastStarted)
	end, begin := finished.Value.AsTime(), started.Value.AsTime()
	return crashRun{restarts: int(count), finished: end,
		short: !begin.IsZero() && !end.Before(begin) &&
			end.Sub(begin) < shortRun}, true
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
	if len(r.read) == 0 && len(r.retryAt) == 0 && len(r.probes) == 0 {
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
	for id := range r.probes {
		if !keep[id] {
			delete(r.probes, id)
		}
	}
}

// noteError logs a refused or failed read once per streak.
func (r *CrashLogRound) noteError(err error) {
	if r.denied.Load() {
		return
	}
	forbidden := apierrors.IsForbidden(err)
	r.denied.Store(forbidden)
	klog.V(1).InfoS("crash log unavailable", "component", "crash-log",
		"forbidden", forbidden, "error", err)
}
