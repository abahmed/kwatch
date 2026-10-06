package detectors

import (
	"strconv"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// Probe failure thresholds. The kubelet records one Unhealthy event per
// failed probe, so probeFailureEvents matches the default failureThreshold.
const (
	unhealthyEvent     = "Unhealthy"
	probeFailureEvents = 3
	probeSustain       = 2 * time.Minute
	probeRecency       = 5 * time.Minute
	// probeNotReadyRecency is how old the newest Unhealthy event of a
	// container that is still not ready may be. The kubelet's event
	// spam filter lets one event per five minutes through after a
	// burst, so a steady failure shows events that are minutes apart.
	probeNotReadyRecency = 15 * time.Minute
)

// probeKinds maps the kubelet's Unhealthy message prefix ("<Type> probe
// failed: ..." or "<Type> probe errored ...", pkg/kubelet/prober) to the
// finding it becomes.
var probeKinds = []struct {
	prefix  string
	reason  string
	summary string
}{
	{"Liveness probe", reasons.LivenessProbeFailed,
		"keeps failing its liveness probe"},
	{"Readiness probe", reasons.ReadinessProbeFailed,
		"keeps failing its readiness probe"},
	{"Startup probe", reasons.StartupProbeFailed,
		"keeps failing its startup probe"},
}

// probeFindings reports a running container whose probe keeps failing,
// before restarts accumulate into HighRestartCount. A failure counts when
// it repeated probeFailureEvents times or the container has been unready
// for probeSustain. Readiness and startup failures inside the container's
// startup budget, and probes failing while the pod shuts down, are
// expected.
func probeFindings(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	if ctx.Model == nil {
		return nil
	}
	note, ok := latestUnhealthy(ctx, e.ID, probeNotReadyRecency)
	if !ok {
		return nil
	}
	for _, kind := range probeKinds {
		if !strings.HasPrefix(note.Message, kind.prefix) {
			continue
		}
		keep, ok := probeVisible(ctx, e, note, kind.reason)
		if !ok {
			return nil
		}
		ctx.RecheckAfter(note.At.Add(keep).Sub(ctx.Now) + time.Nanosecond)
		return []detection.Finding{{
			Reason: kind.reason, Severity: detection.Warning,
			Since:    note.At,
			Summary:  containerRole(e) + " " + kind.summary,
			Evidence: probeEvidence(note),
		}}
	}
	return nil
}

// probeVisible decides whether a probe failure is reported now, and for
// how long the note keeps it alive. Only a readiness failure outlives
// probeRecency, and only while the container is still not ready.
func probeVisible(
	ctx detection.Context, e inventory.Entity, note inventory.Note,
	reason string,
) (time.Duration, bool) {
	liveness := reason == reasons.LivenessProbeFailed
	readiness := reason == reasons.ReadinessProbeFailed
	_, readyKnown := e.Attribute(kube.AttrReady)
	keep := probeRecency
	if readiness && readyKnown && !flag(e, kube.AttrReady) {
		keep = probeNotReadyRecency
	}
	if ctx.Now.Sub(note.At) > keep ||
		(readiness && flag(e, kube.AttrReady)) ||
		(!liveness && (withinStartupBudget(ctx, e) ||
			podBooting(ctx, e))) ||
		!probeSustained(ctx, e, note) || shuttingDown(ctx, e) ||
		(liveness && oneOffKill(e)) {
		return 0, false
	}
	return keep, true
}

func latestUnhealthy(
	ctx detection.Context, id inventory.EntityID, window time.Duration,
) (inventory.Note, bool) {
	var latest inventory.Note
	found := false
	for _, note := range ctx.Model.Notes(id, ctx.Now.Add(-window)) {
		if note.Warning && note.Reason == unhealthyEvent &&
			(!found || note.At.After(latest.At)) {
			latest, found = note, true
		}
	}
	return latest, found
}

func probeSustained(
	ctx detection.Context, e inventory.Entity, note inventory.Note,
) bool {
	if note.Count >= probeFailureEvents {
		return true
	}
	if _, known := e.Attribute(kube.AttrReady); !known ||
		flag(e, kube.AttrReady) {
		return false
	}
	return sustained(ctx, "probe-not-ready", valueSince(e, kube.AttrReady),
		probeSustain)
}

// withinStartupBudget reports a container still inside the time its own
// startup or readiness probe allows before failing: failures then are
// part of starting, not a problem.
func withinStartupBudget(ctx detection.Context, e inventory.Entity) bool {
	started := timestamp(e, kube.AttrStartedAt)
	seconds, ok := number(e, kube.AttrProbeBudget)
	if started.IsZero() || !ok {
		return false
	}
	budget := time.Duration(seconds) * time.Second
	return !sustained(ctx, "startup-budget", started, budget)
}

func shuttingDown(ctx detection.Context, e inventory.Entity) bool {
	pod, ok := owningPod(ctx, e)
	return ok && flag(pod, kube.AttrDeleting)
}

func probeEvidence(note inventory.Note) []detection.Evidence {
	evidence := []detection.Evidence{{Label: "probe", Value: note.Message}}
	if note.Count > 1 {
		evidence = append(evidence, detection.Evidence{
			Label: "failures", Value: strconv.Itoa(note.Count),
		})
	}
	return evidence
}

// podBooting reports a container whose pod starts in a booting node
// pool: its readiness and startup probes fail while the pool warms up.
func podBooting(ctx detection.Context, e inventory.Entity) bool {
	pod, ok := owningPod(ctx, e)
	return ok && bootGraceFor(ctx, pod) > 0
}
