package detectors

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A crash-looping container runs for a moment between two back-offs: the
// kubelet starts it, it fails again, and it waits once more. Reported
// only while it waits, the crash loop would clear and come back with
// every back-off. While it runs shortly after an error exit, after
// several restarts or while the loop is reported already, the crash
// loop it was in is kept.
const (
	// crashLoopKey is the onset a reported crash loop keeps, so the run
	// before the count reaches crashLoopRestarts does not clear it.
	crashLoopKey = "crash-loop"
	// crashLoopRestarts is the least restart count that makes a loop:
	// the kubelet's back-off has started doubling by then.
	crashLoopRestarts = 3
	// crashLoopHold is how long a run counts as one between back-offs:
	// a container that crashes on start fails again within seconds. A
	// container whose last run was longer gets twice that run (runHold),
	// up to maxCrashLoopHold. A longer run is judged as running
	// (restartingFinding), and the incident's own healthy wait decides
	// whether the loop is over.
	crashLoopHold = 30 * time.Second
	// maxCrashLoopHold bounds the hold a long last run gives.
	maxCrashLoopHold = 5 * time.Minute
)

// runHold is how long the current run of a crash-looping container
// counts as one between back-offs. It follows the container's own last
// run, not the kubelet's back-off: a kubelet may cap its back-off far
// below the default five minutes (crashLoopBackOff.
// maxContainerRestartPeriod), and a container that crashes after 45
// seconds then spends most of its loop running. Judged by a fixed hold,
// its crash loop would clear on every run and come back on every crash.
func runHold(e inventory.Entity) time.Duration {
	started := timestamp(e, kube.AttrLastStarted)
	finished := timestamp(e, kube.AttrLastFinished)
	if started.IsZero() || finished.Before(started) {
		return crashLoopHold
	}
	return min(max(2*finished.Sub(started), crashLoopHold),
		maxCrashLoopHold)
}

// crashLoopRunning reports the crash loop of a running container that
// is only between two back-offs, with the reason and wording a waiting
// one gets (crashLoopFinding), or nil.
func crashLoopRunning(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	restarts, _ := number(e, kube.AttrRestarts)
	if restarts < 1 ||
		restarts < crashLoopRestarts && !ctx.Ongoing(crashLoopKey) {
		return nil
	}
	code, known := number(e, kube.AttrLastExitCode)
	oom := text(e, kube.AttrLastReason) == reasons.OOMKilled
	if (!known || code == 0) && !oom {
		return nil
	}
	last := timestamp(e, kube.AttrLastFinished)
	started := timestamp(e, kube.AttrStartedAt)
	if last.IsZero() || started.IsZero() ||
		ctx.Now.Sub(last) > restartRecency ||
		ctx.Now.Sub(started) >= runHold(e) {
		return nil
	}
	ctx.RecheckAfter(started.Add(runHold(e)).Sub(ctx.Now))
	keepCrashLoop(ctx)
	return []detection.Finding{crashLoopFinding(e, last)}
}

// keepCrashLoop notes that the evaluation reports a crash loop.
func keepCrashLoop(ctx detection.Context) {
	ctx.Onset(crashLoopKey, time.Time{})
}

// crashLoopFinding is the finding of a crash-looping container that
// failed last at since: a memory kill when the kernel killed it, else
// the crash loop with how it exited.
func crashLoopFinding(e inventory.Entity, since time.Time) detection.Finding {
	severity, _ := detection.WaitingSeverity(reasons.CrashLoopBackOff)
	s := detection.Finding{
		Reason: reasons.CrashLoopBackOff, Severity: severity, Since: since,
		Summary: containerRole(e) + " " +
			describeWaiting(reasons.CrashLoopBackOff),
		Evidence: containerEvidence(e),
	}
	// A crash loop caused by the kernel OOM killer is an OOM incident;
	// the back-off is only its consequence.
	if text(e, kube.AttrLastReason) == reasons.OOMKilled {
		s.Reason = reasons.OOMKilled
		s.Summary = containerRole(e) + " is killed for exceeding its " +
			"memory limit and restarting"
		return s
	}
	applyCrashLoopExit(&s, e)
	return s
}
