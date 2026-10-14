package detectors

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

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
		withNodeOOM(ctx, e, withHowItFailed(ctx, e, c.byState(ctx, e))))
}

// byState finds what the container's state shows.
func (Container) byState(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	switch text(e, kube.AttrState) {
	case "waiting":
		return withRestartBaseline(ctx, e,
			livenessKilledWaiting(ctx, e, waitingFinding(ctx, e)))
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

func waitingFinding(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	reason := text(e, kube.AttrStateReason)
	severity, failing := detection.WaitingSeverity(reason)
	if !failing {
		return unknownWaiting(ctx, e, reason)
	}
	s := detection.Finding{
		Reason: reason, Severity: severity,
		Since:    valueSince(e, kube.AttrStateReason),
		Summary:  containerRole(e) + " " + describeWaiting(reason),
		Evidence: containerEvidence(e),
	}
	if reason == reasons.CreateConfigError {
		s.Summary = containerRole(e) + " " + configErrorWords(ctx, e)
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

// unknownWaitingGrace is how long a container may wait in a state kwatch
// has no name for before it is reported. Image pulls and volume setup
// are ContainerCreating, which is progress; a state that is neither a
// known failure nor known progress and lasts this long is not assumed
// healthy.
const unknownWaitingGrace = 2 * time.Minute

// unknownWaiting reports a container that has waited in a state the
// registry does not classify. The finding quotes the state's reason and
// message and interprets neither.
func unknownWaiting(
	ctx detection.Context, e inventory.Entity, reason string,
) []detection.Finding {
	if !detection.UnknownWaiting(reason) {
		return nil
	}
	since := valueSince(e, kube.AttrStateReason)
	if !sustained(ctx, "waiting-unknown/"+reason, since,
		unknownWaitingGrace) {
		return nil
	}
	return []detection.Finding{{
		Reason: reasons.UnknownWaiting(reason), Severity: detection.Warning,
		Health:   detection.Failing,
		Mode:     detection.ModeUnknownWaiting,
		Since:    since,
		Summary:  containerRole(e) + " has been waiting in state " + reason,
		Evidence: containerEvidence(e),
	}}
}

func terminatedFinding(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	code, _ := number(e, kube.AttrExitCode)
	reason := text(e, kube.AttrStateReason)
	if code == 0 || reason == reasons.Completed {
		return nil
	}
	if pod, ok := owningPod(ctx, e); ok && ownerJobComplete(ctx, pod) {
		// The Job succeeded on a retry: this run is history, even an OOM.
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
		} else if restarts, _ := number(e, kube.AttrRestarts); restarts > 0 {
			// A container that was restarted before and exits again
			// is in the crash loop the kubelet is about to name. It
			// stays one finding through the seconds it is terminated
			// and the minutes it backs off, so which of the two the
			// first message lands on does not change what it says.
			findingReason = reasons.CrashLoopBackOff
			summary = containerRole(e) + " " +
				describeWaiting(findingReason)
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

// missingRefMessage matches the kubelet's own wording for a Secret or
// ConfigMap that is absent or lacks a referenced key.
var missingRefMessage = regexp.MustCompile(`couldn't find key|` +
	`(secret|configmap) "[^"]*" not found`)

// configErrorWords says why a container's configuration failed. A
// pod that references an object that does not exist is the missing
// reference; anything else (runAsNonRoot with a root image, a bad
// subPath, an invalid env var name) is the kubelet's own rejection,
// whose text the evidence quotes.
func configErrorWords(ctx detection.Context, e inventory.Entity) string {
	const missing = "cannot start: its configuration references " +
		"something missing"
	if pod, ok := owningPod(ctx, e); ok {
		if order, _ := missingObjects(ctx, pod); len(order) > 0 {
			return missing
		}
	}
	// A Secret or ConfigMap that exists without the key, or that the
	// inventory has not synced yet, is named by the kubelet itself.
	if missingRefMessage.MatchString(strings.ToLower(
		text(e, kube.AttrMessage))) {
		return missing
	}
	return "cannot start: the kubelet rejected its configuration"
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
