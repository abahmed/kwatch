package detectors

import (
	"strings"
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
const livenessNoteLead = 2 * time.Minute

// livenessKilled reports a container the kubelet keeps killing for a
// failing liveness probe: its last termination is SIGTERM (exit 143),
// it restarted livenessKills times, the last restart is recent, and a
// liveness "Unhealthy" event is inside the event window. It returns the
// event so the finding can quote it.
func livenessKilled(
	ctx detection.Context, e inventory.Entity,
) (inventory.Note, bool) {
	restarts, _ := number(e, kube.AttrRestarts)
	code, known := number(e, kube.AttrLastExitCode)
	lastRestart := timestamp(e, kube.AttrLastFinished)
	if restarts < livenessKills || !known || code != exitSIGTERM ||
		lastRestart.IsZero() || ctx.Now.Sub(lastRestart) > restartRecency ||
		flag(e, kube.AttrInit) || shuttingDown(ctx, e) || ctx.Model == nil {
		return inventory.Note{}, false
	}
	note, ok := latestUnhealthy(ctx, e.ID, probeRecency)
	if !ok || !isLivenessNote(note) ||
		note.At.Before(lastRestart.Add(-livenessNoteLead)) {
		return inventory.Note{}, false
	}
	return note, true
}

func isLivenessNote(note inventory.Note) bool {
	return strings.HasPrefix(note.Message, livenessPrefix)
}

const livenessPrefix = "Liveness probe"

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
		Summary: containerRole(e) + " keeps being killed by its " +
			"liveness probe",
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
