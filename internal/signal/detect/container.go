package detect

import (
	"strconv"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
	"github.com/abahmed/kwatch/internal/signal"
)

// waitingReasons are container waiting states that are failures. Creating
// and initializing are progress, not failures.
var waitingReasons = map[string]signal.Severity{
	constant.ReasonCrashLoopBackOff:     signal.Critical,
	constant.ReasonImagePullBackOff:     signal.Critical,
	constant.ReasonErrImagePull:         signal.Critical,
	constant.ReasonInvalidImageName:     signal.Critical,
	constant.ReasonImageInspectError:    signal.Critical,
	constant.ReasonCreateConfigError:    signal.Critical,
	constant.ReasonCreateContainerError: signal.Critical,
	"RunContainerError":                 signal.Critical,
	constant.ReasonContainerCannotRun:   signal.Critical,
}

// Container detects failing containers: crash loops (with their last
// termination cause), image and configuration errors, and non-zero exits
// of containers that are not restarted.
type Container struct{}

// Name implements signal.Detector.
func (Container) Name() string { return "container" }

// Kinds implements signal.Detector.
func (Container) Kinds() []knowledge.Kind {
	return []knowledge.Kind{kube.KindContainer}
}

// Detect implements signal.Detector.
func (Container) Detect(_ signal.Context, e knowledge.Entity) []signal.Signal {
	switch text(e, kube.AttrState) {
	case "waiting":
		return waitingSignal(e)
	case "terminated":
		return terminatedSignal(e)
	default:
		return nil
	}
}

func waitingSignal(e knowledge.Entity) []signal.Signal {
	reason := text(e, kube.AttrStateReason)
	severity, failing := waitingReasons[reason]
	if !failing {
		return nil
	}
	s := signal.Signal{
		Reason: reason, Severity: severity,
		Since:    valueSince(e, kube.AttrStateReason),
		Summary:  containerRole(e) + " " + describeWaiting(reason),
		Evidence: containerEvidence(e),
	}
	// A crash loop caused by the kernel OOM killer is an OOM problem; the
	// back-off is only its consequence.
	if reason == constant.ReasonCrashLoopBackOff &&
		text(e, kube.AttrLastReason) == constant.ReasonOOMKilled {
		s.Reason = constant.ReasonOOMKilled
		s.Summary = containerRole(e) + " is killed for exceeding its " +
			"memory limit and restarting"
	}
	return []signal.Signal{s}
}

func terminatedSignal(e knowledge.Entity) []signal.Signal {
	code, _ := number(e, kube.AttrExitCode)
	reason := text(e, kube.AttrStateReason)
	if code == 0 || reason == constant.ReasonCompleted {
		return nil
	}
	signalReason := constant.ReasonError
	switch {
	case reason == constant.ReasonOOMKilled:
		signalReason = constant.ReasonOOMKilled
	case flag(e, kube.AttrInit):
		signalReason = constant.ReasonInitContainerError
	}
	return []signal.Signal{{
		Reason: signalReason, Severity: signal.Critical,
		Since: valueSince(e, kube.AttrState),
		Summary: containerRole(e) + " exited with code " +
			strconv.Itoa(int(code)),
		Evidence: containerEvidence(e),
	}}
}

func describeWaiting(reason string) string {
	switch reason {
	case constant.ReasonCrashLoopBackOff:
		return "keeps crashing and is restarting with back-off"
	case constant.ReasonImagePullBackOff, constant.ReasonErrImagePull:
		return "cannot pull its image"
	case constant.ReasonInvalidImageName:
		return "has an invalid image name"
	case constant.ReasonCreateConfigError:
		return "cannot start: its configuration references something " +
			"missing"
	default:
		return "cannot start (" + reason + ")"
	}
}

func containerRole(e knowledge.Entity) string {
	switch {
	case flag(e, kube.AttrSidecar):
		return "Sidecar container"
	case flag(e, kube.AttrInit):
		return "Init container"
	default:
		return "Container"
	}
}

func containerEvidence(e knowledge.Entity) []signal.Evidence {
	var out []signal.Evidence
	add := func(label, value string) {
		if value != "" {
			out = append(out, signal.Evidence{Label: label, Value: value})
		}
	}
	add("error", text(e, kube.AttrLastMessage))
	add("message", text(e, kube.AttrMessage))
	add("last termination", text(e, kube.AttrLastReason))
	if code, ok := number(e, kube.AttrLastExitCode); ok {
		add("last exit code", strconv.Itoa(int(code)))
	}
	if restarts, ok := number(e, kube.AttrRestarts); ok && restarts > 0 {
		add("restarts", strconv.Itoa(int(restarts)))
	}
	add("image", text(e, kube.AttrImage))
	return out
}
