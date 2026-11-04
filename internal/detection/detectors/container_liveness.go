package detectors

import (
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// livenessKills is how many restarts make liveness kills a loop: one
// kill is a probe doing its job, three are a crash loop.
const livenessKills = 3

// livenessNoteLead is how long before the last kill a liveness
// "Unhealthy" event may be and still explain it. An older one belongs to
// an earlier kill; this kill (a SIGTERM from a rollout or a node drain,
// say) has no probe behind it.
const livenessNoteLead = kube.LivenessKillLead

// livenessKilled reports a container the kubelet keeps killing for a
// failing liveness probe: its last termination is a kill (killExit, or
// any code after the kubelet's liveness stop: livenessKillAtExit),
// it restarted livenessKills times, the last restart is recent, and a
// liveness "Unhealthy" event is inside the event window. It returns the
// event so the finding can quote it.
func livenessKilled(
	ctx detection.Context, e inventory.Entity,
) (inventory.Note, bool) {
	restarts, _ := number(e, kube.AttrRestarts)
	code, known := number(e, kube.AttrLastExitCode)
	lastRestart := timestamp(e, kube.AttrLastFinished)
	killed := known && (killExit(e, code) || livenessKillAtExit(ctx, e))
	if restarts < livenessKills || !killed ||
		lastRestart.IsZero() || ctx.Now.Sub(lastRestart) > restartRecency ||
		flag(e, kube.AttrInit) || shuttingDown(ctx, e) || ctx.Model == nil {
		return inventory.Note{}, false
	}
	// The newest liveness event: a readiness failure after it is its
	// own note and does not hide the kill.
	note, ok := latestProbeNote(ctx.Model.Notes(e.ID,
		ctx.Now.Add(-probeRecency)), livenessPrefix)
	if !ok {
		// An event that names no container belongs to the pod.
		if pod, found := owningPod(ctx, e); found {
			note, ok = latestProbeNote(ctx.Model.Notes(pod.ID,
				ctx.Now.Add(-probeRecency)), livenessPrefix)
		}
	}
	if !ok ||
		note.At.Before(lastRestart.Add(-livenessNoteLead)) {
		return inventory.Note{}, false
	}
	return note, true
}

const livenessPrefix = "Liveness probe"

// livenessKillAtExit reports a kubelet note on the container or its pod
// that it stopped the container for a failing liveness probe
// (kube.LivenessKillMessage), within livenessNoteLead of the end of its
// last run, or the container's own Warning liveness failure just before
// that end (kube.LivenessFailureKill): that run ended because of the
// probe, whatever its code.
func livenessKillAtExit(ctx detection.Context, e inventory.Entity) bool {
	finished := timestamp(e, kube.AttrLastFinished)
	if finished.IsZero() || ctx.Model == nil {
		return false
	}
	if livenessFailureAtExit(ctx, e, finished) {
		return true
	}
	ids := []inventory.EntityID{e.ID}
	if pod, ok := owningPod(ctx, e); ok {
		ids = append(ids, pod.ID)
	}
	for _, id := range ids {
		for _, note := range ctx.Model.Notes(id,
			finished.Add(-livenessNoteLead)) {
			gap := note.At.Sub(finished).Abs()
			if gap <= livenessNoteLead &&
				kube.LivenessKillMessage(note.Message) {
				return true
			}
		}
	}
	return false
}

// livenessFailureAtExit reports the container's latest Warning liveness
// failure as the cause of the run that ended at finished.
func livenessFailureAtExit(
	ctx detection.Context, e inventory.Entity, finished time.Time,
) bool {
	note, ok := latestProbeNote(ctx.Model.Notes(e.ID,
		finished.Add(-livenessNoteLead)), livenessPrefix)
	if !ok {
		return false
	}
	restarts, _ := number(e, kube.AttrRestarts)
	threshold, _ := number(e, kube.ProbeAttr("liveness", "failures"))
	return kube.LivenessFailureKill(kube.LivenessFailure{
		Message: note.Message, At: note.At, Count: note.Count,
		Threshold: int(threshold), Finished: finished,
		Restarts: int(restarts),
	})
}

// killExit reports an exit the kubelet's stop leaves: SIGTERM (143), or
// SIGKILL (137) when the process did not stop within its grace period.
// A memory kill also ends with 137 but says OOMKilled.
func killExit(e inventory.Entity, code float64) bool {
	return code == exitSIGTERM || code == exitSIGKILL &&
		text(e, kube.AttrLastReason) != reasons.OOMKilled
}

// livenessKilledFinding is the crash-loop-class finding of a running
// container between kills. It replaces the plain probe finding: the
// probe failing is the cause, the loop is what people need to hear.
func livenessKilledFinding(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	note, ok := livenessKilled(ctx, e)
	if !ok {
		return nil
	}
	lastRestart := timestamp(e, kube.AttrLastFinished)
	ctx.RecheckAfter(lastRestart.Add(restartRecency).Sub(ctx.Now))
	return []detection.Finding{
		withStartFacts(ctx, e, livenessFinding(e, note))}
}

// livenessKilledWaiting turns the CrashLoopBackOff finding of a
// container that liveness probes keep killing into the same finding as
// when it runs, so the lead reads the same between kills. The other
// findings pass through.
func livenessKilledWaiting(
	ctx detection.Context, e inventory.Entity, found []detection.Finding,
) []detection.Finding {
	if len(found) != 1 || found[0].Reason != reasons.CrashLoopBackOff {
		return found
	}
	note, ok := livenessKilled(ctx, e)
	if !ok {
		return found
	}
	since := found[0].Since
	killed := withStartFacts(ctx, e, livenessFinding(e, note))
	killed.Since = since
	return []detection.Finding{killed}
}

func livenessFinding(
	e inventory.Entity, note inventory.Note,
) detection.Finding {
	return detection.Finding{
		Reason: reasons.LivenessKilled, Severity: detection.Critical,
		Since: timestamp(e, kube.AttrLastFinished),
		Summary: containerRole(e) + " keeps failing its liveness " +
			"check" + resultWords(note) + ", so the kubelet keeps " +
			"restarting it",
		Evidence: append(append(containerEvidence(e),
			probeEvidence(note)...), throttleEvidence(e)...),
	}
}

// oneOffKill reports a container the liveness probe killed once and
// that is ready again: the probe did its job, and a single restart is
// not a loop.
func oneOffKill(e inventory.Entity) bool {
	restarts, _ := number(e, kube.AttrRestarts)
	code, _ := number(e, kube.AttrLastExitCode)
	return restarts == 1 && code == exitSIGTERM && flag(e, kube.AttrReady)
}
