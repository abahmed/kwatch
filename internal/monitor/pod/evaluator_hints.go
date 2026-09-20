package pod

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/enricher"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/monitor/pod/enrichment"
	"github.com/abahmed/kwatch/internal/observe"
)

func (e *PolicyEvaluator) buildContainerHint(
	ctx *enrichment.Context,
) (string, model.Facts) {
	reason := ctx.Container.Reason
	exitCode := ctx.Container.ExitCode
	hint := hintForReasonAndCode(reason, exitCode, ctx.Container.IsInit)
	spec := findContainerSpec(ctx.Pod, ctx.Container.Container.Name)
	isOOM := reason == constant.ReasonOOMKilled
	if isOOM && e.oomTracker != nil {
		if hint, facts, repeating := e.repeatingOOMHint(ctx); repeating {
			return hint, facts
		}
	}
	var facts model.Facts
	switch {
	case isOOM:
		hint, facts = e.oomHint(spec, ctx)
	case exitCode == 137:
		hint = "Killed (SIGKILL, exit 137) — terminated by something " +
			"other than the OOM killer (check evictions, liveness probes, " +
			"or manual termination)"
	}
	if spec != nil {
		switch reason {
		case constant.ReasonLivenessProbeFailed,
			constant.ReasonReadinessProbeFailed,
			constant.ReasonStartupProbeFailed:
			hint = BuildProbeHint(reason, spec)
			facts.ProbeEndpoint = ProbeEndpoint(reason, spec)
		case constant.ReasonCrashLoopBackOff:
			if spec.LivenessProbe != nil {
				hint += "; check liveness probe configuration"
			}
		}
	}
	if ctx.Container.Msg != "" {
		hint = ctx.Container.Msg + " — " + hint
	}
	hint, facts.PullSecretsSet = appendImagePullHint(
		hint, ctx, reason,
	)
	return hint, facts
}

func hintForReasonAndCode(reason string, exitCode int32, isInit bool) string {
	hint := enricher.HintForReason(reason)
	exitHint := enricher.HintForExitCode(exitCode)
	if isInit && exitCode != 0 {
		hint = enricher.HintForReason(constant.ReasonInitContainerError)
		if exitHint != "" {
			hint = enricher.CombineHints(hint, exitHint)
		}
	} else if exitCode != 0 && exitHint != "" {
		hint = enricher.CombineHints(hint, exitHint)
	}
	return hint
}

func (e *PolicyEvaluator) repeatingOOMHint(
	ctx *enrichment.Context,
) (string, model.Facts, bool) {
	key := e.oomKey(ctx)
	count, repeating := e.oomTracker.record(key)
	if !repeating {
		return "", model.Facts{}, false
	}
	ctx.Container.Reason = constant.ReasonOOMRepeating
	facts := model.Facts{
		OOMCount:     count,
		OOMWindowMin: e.runtime.Monitors().OOM().WindowMinutes,
		MemoryLeak:   true,
	}
	if timeline := e.oomTracker.history(key); timeline != "" {
		facts.OOMTimeline = "[" + timeline + "]"
		return fmt.Sprintf(
			"OOMKilled %d times in %dm — potential memory leak [%s]",
			count, e.runtime.Monitors().OOM().WindowMinutes, timeline,
		), facts, true
	}
	return fmt.Sprintf(
		"OOMKilled %d times in %dm — potential memory leak",
		count, e.runtime.Monitors().OOM().WindowMinutes,
	), facts, true
}

func (e *PolicyEvaluator) oomHint(
	spec *corev1.Container,
	ctx *enrichment.Context,
) (string, model.Facts) {
	hint := "OOMKilled with no memory limit set — node-level memory " +
		"pressure; set/raise container memory limits"
	var facts model.Facts
	if spec != nil && spec.Resources.Limits != nil {
		memory := spec.Resources.Limits.Memory()
		if memory != nil && !memory.IsZero() {
			facts.MemoryLimit = memory.String()
			hint = fmt.Sprintf(
				"OOMKilled (memory limit: %s) — consider increasing memory "+
					"limits", memory.String(),
			)
		}
	}
	if e.oomTracker != nil {
		if timeline := e.oomTracker.history(e.oomKey(ctx)); timeline != "" {
			facts.OOMTimeline = "[" + timeline + "]"
			hint += " [" + timeline + "]"
		}
	}
	return hint, facts
}

func (e *PolicyEvaluator) oomKey(ctx *enrichment.Context) string {
	owner := contextOwner(ctx)
	if owner.Name != "" {
		return ctx.Pod.Namespace + "/" + owner.Name + "/" +
			ctx.Container.Container.Name
	}
	return ctx.Pod.Namespace + "/" + ctx.Pod.Name + "/" +
		ctx.Container.Container.Name
}

func (e *PolicyEvaluator) emitHighRestartAlert(
	ctx *enrichment.Context,
	container *corev1.ContainerStatus,
) {
	owner := contextOwner(ctx)
	if owner.Name == "" {
		return
	}
	lastReason, lastExitCode := lastTermination(container)
	obs := observe.PodOwnedBy(
		ctx.Pod, container.Name, constant.ReasonHighRestartCount, owner,
	).WithHint(fmt.Sprintf(
		"container restarted %d times (last exit: %s, code %d)",
		container.RestartCount, lastReason, lastExitCode,
	)).WithEvidence("", "", &model.ContainerState{
		RestartCount: container.RestartCount,
		Reason:       lastReason,
		ExitCode:     lastExitCode,
	})
	obs.Image = container.Image
	obs.RestartCount = container.RestartCount
	e.observe(obs)
}

func appendImagePullHint(
	hint string,
	ctx *enrichment.Context,
	reason string,
) (string, bool) {
	if (reason != constant.ReasonImagePullBackOff &&
		reason != constant.ReasonErrImagePull) || ctx.Pod == nil {
		return hint, false
	}
	hasSecrets := len(ctx.Pod.Spec.ImagePullSecrets) > 0
	messageHint := ImagePullMessageHint(ctx.Container.Msg, hasSecrets)
	if messageHint != "" {
		return hint + "; " + messageHint, hasSecrets
	}
	if !hasSecrets {
		for _, container := range ctx.Pod.Spec.Containers {
			if container.Name == ctx.Container.Container.Name &&
				NeedsRegistryAuth(container.Image) {
				return hint + "; this image is from a registry that " +
					"typically requires authentication — add imagePullSecrets " +
					"to the pod spec", false
			}
		}
		return hint, false
	}
	return hint + "; imagePullSecrets is configured — check the image " +
		"name/tag or secret validity", true
}

func findContainerSpec(pod *corev1.Pod, name string) *corev1.Container {
	for index := range pod.Spec.Containers {
		if pod.Spec.Containers[index].Name == name {
			return &pod.Spec.Containers[index]
		}
	}
	for index := range pod.Spec.InitContainers {
		if pod.Spec.InitContainers[index].Name == name {
			return &pod.Spec.InitContainers[index]
		}
	}
	return nil
}

func lastTermination(
	container *corev1.ContainerStatus,
) (reason string, exitCode int32) {
	if terminated := container.LastTerminationState.Terminated; terminated != nil {
		return terminated.Reason, terminated.ExitCode
	}
	return "", 0
}
