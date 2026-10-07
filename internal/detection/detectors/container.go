package detectors

import (
	"strconv"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// waitingReasons are container waiting states that are failures. Creating
// and initializing are progress, not failures.
var waitingReasons = map[string]detection.Severity{
	reasons.CrashLoopBackOff:     detection.Critical,
	reasons.ImagePullBackOff:     detection.Critical,
	reasons.ErrImagePull:         detection.Critical,
	reasons.InvalidImageName:     detection.Critical,
	reasons.ImageInspectError:    detection.Critical,
	reasons.CreateConfigError:    detection.Critical,
	reasons.CreateContainerError: detection.Critical,
	"RunContainerError":          detection.Critical,
	reasons.ContainerCannotRun:   detection.Critical,
	reasons.ErrImageNeverPull:    detection.Critical,
	reasons.PreStartHookError:    detection.Critical,
	reasons.PostStartHookError:   detection.Critical,
}

// Container detects failing containers: crash loops (with their last
// termination cause), image and configuration errors, and non-zero exits
// of containers that are not restarted.
type Container struct{}

// Name implements detection.Detector.
func (Container) Name() string { return "container" }

// Kinds implements detection.Detector.
func (Container) Kinds() []inventory.Kind {
	return []inventory.Kind{kube.KindContainer}
}

// Detect implements detection.Detector.
func (c Container) Detect(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	return withArchMismatch(ctx, e,
		withNodeOOM(ctx, e, c.byState(ctx, e)))
}

// byState finds what the container's state shows.
func (Container) byState(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	switch text(e, kube.AttrState) {
	case "waiting":
		return withRestartBaseline(ctx, e,
			livenessKilledWaiting(ctx, e, waitingFinding(e)))
	case "terminated":
		return terminatedFinding(ctx, e)
	case "running":
		if killed := livenessKilledFinding(ctx, e); killed != nil {
			return killed
		}
		return append(withRestartBaseline(ctx, e,
			restartingFinding(ctx, e)), probeFindings(ctx, e)...)
	default:
		return nil
	}
}

// Restart thresholds for running containers.
const (
	highRestarts     = 5
	restartRecency   = 15 * time.Minute
	repeatedOOMCount = 3
)

// restartingFinding reports a container that is running now but was
// recently restarted again after several restarts, typically killed by
// its liveness probe or the OOM killer between back-offs.
func restartingFinding(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	restarts, _ := number(e, kube.AttrRestarts)
	if restarts < highRestarts {
		return nil
	}
	// The last termination's finish time is when the container last
	// restarted. The attribute's own Since is only when kwatch first saw
	// the count, so after a kwatch restart old restarts would look new.
	lastRestart := timestamp(e, kube.AttrLastFinished)
	if lastRestart.IsZero() || ctx.Now.Sub(lastRestart) > restartRecency {
		return nil
	}
	ctx.RecheckAfter(lastRestart.Add(restartRecency).Sub(ctx.Now))
	// The restart count is evidence, not part of the stable summary.
	if text(e, kube.AttrLastReason) == reasons.OOMKilled &&
		restarts >= repeatedOOMCount {
		return []detection.Finding{{
			Reason: reasons.OOMKilled, Severity: detection.Critical,
			Since: lastRestart,
			Summary: containerRole(e) + " is repeatedly killed for " +
				"exceeding its memory limit",
			Evidence: containerEvidence(e),
		}}
	}
	return []detection.Finding{{
		Reason: reasons.HighRestartCount, Severity: detection.Warning,
		Since:    lastRestart,
		Summary:  containerRole(e) + " keeps restarting",
		Evidence: containerEvidence(e),
	}}
}

func waitingFinding(e inventory.Entity) []detection.Finding {
	reason := text(e, kube.AttrStateReason)
	severity, failing := waitingReasons[reason]
	if !failing {
		return nil
	}
	s := detection.Finding{
		Reason: reason, Severity: severity,
		Since:    valueSince(e, kube.AttrStateReason),
		Summary:  containerRole(e) + " " + describeWaiting(reason),
		Evidence: containerEvidence(e),
	}
	// A crash loop caused by the kernel OOM killer is an OOM incident; the
	// back-off is only its consequence.
	if reason == reasons.CrashLoopBackOff &&
		text(e, kube.AttrLastReason) == reasons.OOMKilled {
		s.Reason = reasons.OOMKilled
		s.Summary = containerRole(e) + " is killed for exceeding its " +
			"memory limit and restarting"
	} else if reason == reasons.CrashLoopBackOff {
		applyCrashLoopExit(&s, e)
	}
	// The restart count stays in the evidence: a summary that changed
	// with every restart would report the finding as changed each time.
	return []detection.Finding{s}
}

// Exit codes of containers stopped by a finding: 128+SIGTERM and
// 128+SIGKILL (after the grace period, when not OOMKilled).
const (
	exitSIGTERM = 143
	exitSIGKILL = 137
)

func terminatedFinding(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	code, _ := number(e, kube.AttrExitCode)
	reason := text(e, kube.AttrStateReason)
	if code == 0 || reason == reasons.Completed {
		return nil
	}
	if reason != reasons.OOMKilled && stoppedOnPurpose(ctx, e, code) {
		return nil
	}
	findingReason := reasons.Error
	summary := containerRole(e) + " exited with code " +
		strconv.Itoa(int(code))
	switch {
	case reason == reasons.OOMKilled:
		findingReason = reasons.OOMKilled
	case flag(e, kube.AttrInit):
		findingReason = reasons.InitContainerError
	default:
		if exit, ok := terminatedExit(code, reason); ok {
			findingReason = exit.reason
			summary = containerRole(e) + " " + exit.summary
		}
	}
	return []detection.Finding{{
		Reason: findingReason, Severity: detection.Critical,
		Since:    valueSince(e, kube.AttrState),
		Summary:  summary,
		Evidence: containerEvidence(e),
	}}
}

// stoppedOnPurpose reports a termination Kubernetes caused: any exit of a
// container whose pod completed, is being deleted or is a disruption
// target, and a sidecar stopped by SIGTERM or SIGKILL, which is how
// sidecars end when a Job's main container finishes.
func stoppedOnPurpose(
	ctx detection.Context, e inventory.Entity, code float64,
) bool {
	if flag(e, kube.AttrSidecar) &&
		(code == exitSIGTERM || code == exitSIGKILL) {
		return true
	}
	pod, ok := owningPod(ctx, e)
	if !ok {
		return false
	}
	return text(pod, kube.AttrPhase) == "Succeeded" ||
		flag(pod, kube.AttrDeleting) || disrupted(pod)
}

func owningPod(
	ctx detection.Context, e inventory.Entity,
) (inventory.Entity, bool) {
	if ctx.Model == nil {
		return inventory.Entity{}, false
	}
	pods := ctx.Model.Related(e.ID, inventory.PartOf, inventory.Outgoing)
	if len(pods) == 0 {
		return inventory.Entity{}, false
	}
	return ctx.Model.Entity(pods[0])
}

func describeWaiting(reason string) string {
	switch reason {
	case reasons.CrashLoopBackOff:
		return "keeps crashing and is restarting with back-off"
	case reasons.ImagePullBackOff, reasons.ErrImagePull:
		return "cannot pull its image"
	case reasons.InvalidImageName:
		return "has an invalid image name"
	case reasons.CreateConfigError:
		return "cannot start: its configuration references something " +
			"missing"
	case reasons.ErrImageNeverPull:
		return "cannot start: its image is not on the node and the " +
			"pull policy is Never"
	case reasons.PreStartHookError:
		return "cannot start: the kubelet pre-start hook failed"
	case reasons.PostStartHookError:
		return "was stopped because its postStart hook failed"
	default:
		return "cannot start (" + reason + ")"
	}
}

func containerRole(e inventory.Entity) string {
	switch {
	case flag(e, kube.AttrSidecar):
		return "Sidecar container"
	case flag(e, kube.AttrInit):
		return "Init container"
	default:
		return "Container"
	}
}

func containerEvidence(e inventory.Entity) []detection.Evidence {
	var out []detection.Evidence
	add := func(label, value string) {
		if value != "" {
			out = append(out, detection.Evidence{Label: label, Value: value})
		}
	}
	// The termination message is the container's own last word; a
	// container killed by a probe leaves it empty, and the first error
	// line of its previous log stands in. Either is quoted as is.
	errorText := text(e, kube.AttrLastMessage)
	if errorText == "" {
		errorText = text(e, kube.AttrLastErrorLine)
	}
	add(detection.EvidenceError, errorText)
	add("message", text(e, kube.AttrMessage))
	add("last termination", text(e, kube.AttrLastReason))
	if code, ok := number(e, kube.AttrLastExitCode); ok {
		add("last exit code", strconv.Itoa(int(code)))
	}
	if restarts, ok := number(e, kube.AttrRestarts); ok && restarts > 0 {
		add("restarts", strconv.Itoa(int(restarts)))
	}
	add("image", text(e, kube.AttrImage))
	if killedByMemory(e) {
		out = append(out, memoryEvidence(e)...)
	}
	return out
}
