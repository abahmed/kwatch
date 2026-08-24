package handler

import (
	"fmt"
	"strings"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/enricher"
	"github.com/abahmed/kwatch/internal/filter"

	corev1 "k8s.io/api/core/v1"
)

// buildContainerHint computes a rich diagnostic hint from container state + spec.
func (h *handler) buildContainerHint(ctx *filter.Context) string {
	reason := ctx.Container.Reason
	exitCode := ctx.Container.ExitCode

	hint := hintForReasonAndCode(reason, exitCode, ctx.Container.IsInit)
	spec := findContainerSpec(ctx.Pod, ctx.Container.Container.Name)

	// Exit code 137 alone is not proof of an OOM kill — SIGKILL (manual kill,
	// eviction, liveness-probe kill) produces the same code. Only the kubelet's
	// OOMKilled reason is treated as memory pressure; 137 with any other reason
	// is labeled as a plain SIGKILL ("Killed").
	isOOM := reason == constant.ReasonOOMKilled

	if isOOM && h.oomTracker != nil {
		if s, repeating := h.repeatingOOMHint(ctx); repeating {
			return s
		}
	}

	switch {
	case isOOM:
		hint = h.oomHint(spec, ctx)
	case exitCode == 137:
		hint = "Killed (SIGKILL, exit 137) — terminated by something other than the OOM killer (check evictions, liveness probes, or manual termination)"
	}

	if spec != nil {
		if reason == constant.ReasonLivenessProbeFailed || reason == constant.ReasonReadinessProbeFailed || reason == constant.ReasonStartupProbeFailed {
			hint = buildProbeHint(reason, spec)
		} else if reason == constant.ReasonCrashLoopBackOff && spec.LivenessProbe != nil {
			hint += "; check liveness probe configuration"
		}
	}

	// Prepend the K8s container message when available — it has the
	// most specific diagnostic info (e.g., "Back-off pulling image ...").
	if ctx.Container.Msg != "" {
		hint = ctx.Container.Msg + " — " + hint
	}

	return h.appendImagePullHint(hint, ctx, reason)
}

// hintForReasonAndCode combines the reason hint with exit-code guidance,
// treating a failed init container as its own error class.
func hintForReasonAndCode(reason string, exitCode int32, isInit bool) string {
	hint := enricher.HintForReason(reason)
	ecHint := enricher.HintForExitCode(exitCode)
	if isInit && exitCode != 0 {
		hint = enricher.HintForReason(constant.ReasonInitContainerError)
		if ecHint != "" {
			hint = enricher.CombineHints(hint, ecHint)
		}
	} else if exitCode != 0 && ecHint != "" {
		hint = enricher.CombineHints(hint, ecHint)
	}
	return hint
}

// repeatingOOMHint records another OOM event and reports the leak signal once
// the container exceeds the repetition threshold.
func (h *handler) repeatingOOMHint(ctx *filter.Context) (string, bool) {
	key := h.oomKey(ctx)
	count, repeating := h.oomTracker.record(key)
	if !repeating {
		return "", false
	}
	ctx.Container.Reason = constant.ReasonOOMRepeating
	timeline := h.oomTracker.History(key)
	if timeline != "" {
		return fmt.Sprintf("OOMKilled %d times in %dm — potential memory leak [%s]", count, h.config.OomMonitor.WindowMinutes, timeline), true
	}
	return fmt.Sprintf("OOMKilled %d times in %dm — potential memory leak", count, h.config.OomMonitor.WindowMinutes), true
}

// oomHint explains whether the kill came from a configured limit or from
// node-level pressure when no limit is set.
func (h *handler) oomHint(spec *corev1.Container, ctx *filter.Context) string {
	noLimit := "OOMKilled with no memory limit set — node-level memory pressure; set/raise container memory limits"
	hint := ""
	if spec != nil && spec.Resources.Limits != nil {
		mem := spec.Resources.Limits.Memory()
		switch {
		case mem != nil && !mem.IsZero():
			hint = fmt.Sprintf("OOMKilled (memory limit: %s) — consider increasing memory limits", mem.String())
		default:
			hint = noLimit
		}
	} else {
		hint = noLimit
	}
	if h.oomTracker != nil {
		if timeline := h.oomTracker.History(h.oomKey(ctx)); timeline != "" {
			hint = hint + " [" + timeline + "]"
		}
	}
	return hint
}

// appendImagePullHint adds registry-specific guidance for image pull failures.
func (h *handler) appendImagePullHint(hint string, ctx *filter.Context, reason string) string {
	if (reason != constant.ReasonImagePullBackOff && reason != constant.ReasonErrImagePull) || ctx.Pod == nil {
		return hint
	}

	hasSecrets := len(ctx.Pod.Spec.ImagePullSecrets) > 0

	if m := imagePullMsgHint(ctx.Container.Msg, hasSecrets); m != "" {
		return hint + "; " + m
	}
	if !hasSecrets {
		// No specific pattern — check if registry likely needs auth.
		for _, c := range ctx.Pod.Spec.Containers {
			if c.Name == ctx.Container.Container.Name && needsRegistryAuth(c.Image) {
				return hint + "; this image is from a registry that typically requires " +
					"authentication — add imagePullSecrets to the pod spec"
			}
		}
		return hint
	}
	return hint + "; imagePullSecrets is configured — check the image name/tag or secret validity"
}

func buildProbeHint(reason string, spec *corev1.Container) string {
	var probe *corev1.Probe
	switch reason {
	case constant.ReasonLivenessProbeFailed:
		probe = spec.LivenessProbe
	case constant.ReasonReadinessProbeFailed:
		probe = spec.ReadinessProbe
	case constant.ReasonStartupProbeFailed:
		probe = spec.StartupProbe
	}
	if probe == nil {
		return enricher.HintForReason(reason)
	}

	detail := reason
	switch {
	case probe.HTTPGet != nil:
		detail = fmt.Sprintf("%s (HTTP GET http://%s%s:%d%s)", reason, spec.Name, probe.HTTPGet.Host, probe.HTTPGet.Port.IntValue(), probe.HTTPGet.Path)
	case probe.TCPSocket != nil:
		detail = fmt.Sprintf("%s (TCP check :%d)", reason, probe.TCPSocket.Port.IntValue())
	case probe.Exec != nil:
		cmd := ""
		if len(probe.Exec.Command) > 0 {
			cmd = probe.Exec.Command[0]
		}
		detail = fmt.Sprintf("%s (exec %s)", reason, cmd)
	}
	return fmt.Sprintf("%s — application not responding to %s probe", detail, probeType(reason))
}

func probeType(reason string) string {
	switch reason {
	case constant.ReasonLivenessProbeFailed:
		return "liveness"
	case constant.ReasonReadinessProbeFailed:
		return "readiness"
	case constant.ReasonStartupProbeFailed:
		return "startup"
	}
	return "probe"
}

func lastTermInfo(container *corev1.ContainerStatus) (reason string, exitCode int32) {
	if last := container.LastTerminationState.Terminated; last != nil {
		return last.Reason, last.ExitCode
	}
	return "", 0
}

// imagePullPattern matches substrings of the kubelet image-pull error message.
// hintWithSecrets, when set, replaces hint for registries configured with
// imagePullSecrets.
type imagePullPattern struct {
	match           []string
	hint            string
	hintWithSecrets string
}

var imagePullPatterns = []imagePullPattern{
	{
		match: []string{"toomanyrequests", "rate limit"},
		hint:  "Docker Hub rate limit exceeded — add imagePullSecrets for authenticated pulls or configure a mirror registry",
	},
	{
		match: []string{"pull qps"},
		hint:  "Kubelet image pull QPS limit exceeded — consider increasing registryPullQPS in kubelet config or reducing concurrent pod starts",
	},
	{
		match: []string{"authentication required", "unauthorized", "denied", "no pull access"},
		hint:  "Registry authentication failed — check imagePullSecrets validity",
	},
	{
		match:           []string{"not found", "manifest unknown", "does not exist"},
		hint:            "Image not found — check the image name/tag",
		hintWithSecrets: "Image not found — check the image name/tag, or the image may not exist in this registry",
	},
	{
		match: []string{"context deadline exceeded", "i/o timeout"},
		hint:  "Registry connection timed out — check network connectivity to the registry and DNS resolution",
	},
	{
		match: []string{"connection refused", "connection reset"},
		hint:  "Registry connection refused — check that the registry is running and not blocked by a firewall",
	},
	{
		match: []string{"no route to host", "network is unreachable"},
		hint:  "No network route to registry — check firewall rules and network connectivity",
	},
	{
		match: []string{"no such host", "dial tcp"},
		hint:  "Registry unreachable — check cluster network connectivity and DNS",
	},
	{
		match: []string{"tls", "certificate"},
		hint:  "Registry TLS error — check registry certificate or configure insecure-registries",
	},
}

// imagePullMsgHint returns a targeted fix suggestion when the image-pull
// error message matches a well-known pattern such as rate limiting or
// authentication failure.  Returns "" when no pattern matches.
func imagePullMsgHint(msg string, hasSecrets bool) string {
	msg = strings.ToLower(msg)
	for _, p := range imagePullPatterns {
		if !containsAny(msg, p.match) {
			continue
		}
		if hasSecrets && p.hintWithSecrets != "" {
			return p.hintWithSecrets
		}
		return p.hint
	}
	return ""
}

func containsAny(msg string, patterns []string) bool {
	for _, p := range patterns {
		if strings.Contains(msg, p) {
			return true
		}
	}
	return false
}

// needsRegistryAuth returns true when the image is hosted on a registry that
// almost always requires pull credentials (gcr.io, ECR, ACR, Quay, GHCR, etc.).
// Official Docker Hub images (e.g., "nginx", "nginx:latest") have no "/" and
// never need auth.  User images ("user/repo:tag") are ambiguous but common
// public repos don't need credentials, so only explicit-registry images
// (host contains "." or ":") are flagged.
func needsRegistryAuth(image string) bool {
	// Images without "/" are always official Docker Hub (library/) — no auth.
	slash := strings.IndexByte(image, '/')
	if slash < 0 {
		return false
	}
	host := image[:slash]
	return strings.Contains(host, ".") || strings.Contains(host, ":")
}
