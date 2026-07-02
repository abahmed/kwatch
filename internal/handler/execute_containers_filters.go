package handler

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/correlation"
	"github.com/abahmed/kwatch/internal/enricher"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/filter"
	"github.com/abahmed/kwatch/internal/k8s"
	"github.com/abahmed/kwatch/internal/model"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog/v2"
)

func (h *handler) executeContainersFilters(ctx *filter.Context) {
	containers := make([]*corev1.ContainerStatus, 0)
	containerIsInit := make(map[string]bool)
	for idx := range ctx.Pod.Status.InitContainerStatuses {
		c := &ctx.Pod.Status.InitContainerStatuses[idx]
		containers = append(containers, c)
		containerIsInit[c.Name] = true
	}
	for idx := range ctx.Pod.Status.ContainerStatuses {
		containers = append(containers, &ctx.Pod.Status.ContainerStatuses[idx])
	}

	for _, container := range containers {
		ctx.Container = &filter.ContainerContext{
			Container:        container,
			HasRestarts:      false,
			LastTerminatedOn: time.Time{},
			LastState: h.correlator.GetLastContainerState(
				ctx.Pod.Namespace, ctx.Pod.Name, container.Name),
			IsInit: containerIsInit[container.Name],
		}

		// Phase 1: Detect (pure, no I/O)
		broken := false
		for i := range h.containerDetectors {
			if h.containerDetectors[i].Detect(ctx) == filter.StatusSkip {
				broken = false
				break
			}
			broken = true
		}

		if !broken {
			if th := h.config.ContainerRestartThreshold; th > 0 &&
				int(container.RestartCount) >= th &&
				!isPodTerminatingOrDisrupted(ctx.Pod) {
				h.emitHighRestartAlert(ctx, container)
			}
			continue
		}

		// Phase 2: Enrich (I/O: events, owner, logs)
		if ctx.Events == nil {
			if ctx.EventLister != nil {
				all, err := ctx.EventLister.Events(ctx.Pod.Namespace).List(labels.Everything())
				if err != nil {
					klog.ErrorS(err, "event lister failed", "pod", ctx.Pod.Name)
				} else {
					items := make([]corev1.Event, 0, len(all))
					for _, e := range all {
						if e.InvolvedObject.Kind == "Pod" && e.InvolvedObject.Name == ctx.Pod.Name {
							items = append(items, *e)
						}
					}
					sort.Slice(items, func(i, j int) bool {
						return items[i].LastTimestamp.Before(&items[j].LastTimestamp)
					})
					ctx.Events = &items
				}
			} else {
				podEvents, err := k8s.GetPodEvents(ctx.Ctx, ctx.Client, ctx.Pod.Name, ctx.Pod.Namespace)
				if err != nil {
					klog.ErrorS(err, "failed to fetch pod events", "pod", ctx.Pod.Name)
				}
				if podEvents != nil {
					ctx.Events = &podEvents.Items
				}
			}
		}

		for i := range h.containerEnrichers {
			if h.containerEnrichers[i].Enrich(ctx) {
				broken = false
				break
			}
		}

		if !broken {
			continue
		}

		ownerName := ""
		if ctx.Owner != nil {
			ownerName = ctx.Owner.Name
		}

		klog.V(2).InfoS(
			"container only issue",
			"container", ctx.Container.Container.Name,
			"pod", ctx.Pod.Name,
			"owner", ownerName,
			"reason", ctx.Container.Reason,
			"message", ctx.Container.Msg,
			"exitCode", ctx.Container.ExitCode)

		ownerKind := ""
		if ctx.Owner != nil {
			ownerKind = ctx.Owner.Kind
		}

		hint := buildContainerHint(ctx)
		h.signalEvent(&event.Signal{
			Resource:     "pod",
			PodName:      ctx.Pod.Name,
			Container:    ctx.Container.Container.Name,
			Namespace:    ctx.Pod.Namespace,
			NodeName:     ctx.Pod.Spec.NodeName,
			Reason:       ctx.Container.Reason,
			Events:       k8s.GetPodEventsStr(ctx.Events),
			Logs:         ctx.Container.Logs,
			Labels:       ctx.Pod.Labels,
			OwnerKind:    ownerKind,
			RestartCount: ctx.Container.Container.RestartCount,
			Hint:         hint,
			Owner:        ownerName,
			ContainerState: &model.ContainerState{
				RestartCount:     ctx.Container.Container.RestartCount,
				LastTerminatedOn: ctx.Container.LastTerminatedOn,
				Reason:           ctx.Container.Reason,
				Msg:              ctx.Container.Msg,
				ExitCode:         ctx.Container.ExitCode,
				Status:           ctx.Container.Status,
			},
		})
	}
}

// findContainerSpec returns the matching container spec (including init containers) by name.
func findContainerSpec(pod *corev1.Pod, name string) *corev1.Container {
	for i := range pod.Spec.Containers {
		if pod.Spec.Containers[i].Name == name {
			return &pod.Spec.Containers[i]
		}
	}
	for i := range pod.Spec.InitContainers {
		if pod.Spec.InitContainers[i].Name == name {
			return &pod.Spec.InitContainers[i]
		}
	}
	return nil
}

// buildContainerHint computes a rich diagnostic hint from container state + spec.
func buildContainerHint(ctx *filter.Context) string {
	reason := ctx.Container.Reason
	exitCode := ctx.Container.ExitCode

	hint := enricher.HintForReason(reason)

	if ctx.Container.IsInit && exitCode != 0 {
		hint = enricher.HintForReason("InitContainerError")
		if ecHint := enricher.HintForExitCode(exitCode); ecHint != "" {
			hint = enricher.CombineHints(hint, ecHint)
		}
	} else if exitCode != 0 {
		ecHint := enricher.HintForExitCode(exitCode)
		hint = enricher.CombineHints(hint, ecHint)
	}

	spec := findContainerSpec(ctx.Pod, ctx.Container.Container.Name)

	if reason == "OOMKilled" || exitCode == 137 {
		if spec != nil && spec.Resources.Limits != nil {
			mem := spec.Resources.Limits.Memory()
			if mem != nil && !mem.IsZero() {
				hint = fmt.Sprintf("OOMKilled (memory limit: %s) — consider increasing memory limits", mem.String())
			} else {
				hint = "OOMKilled with no memory limit set — node-level memory pressure; set/raise container memory limits"
			}
		} else {
			hint = "OOMKilled with no memory limit set — node-level memory pressure; set/raise container memory limits"
		}
	}

	if spec != nil {
		if reason == "LivenessProbeFailed" || reason == "ReadinessProbeFailed" || reason == "StartupProbeFailed" {
			hint = buildProbeHint(reason, spec)
		} else if reason == "CrashLoopBackOff" && spec.LivenessProbe != nil {
			hint = hint + "; check liveness probe configuration"
		}
	}

	// Prepend the K8s container message when available — it has the
	// most specific diagnostic info (e.g., "Back-off pulling image ...").
	if ctx.Container.Msg != "" {
		hint = ctx.Container.Msg + " — " + hint
	}

	// Smart diagnostics for obvious reasons (no LLM needed).
	if (reason == "ImagePullBackOff" || reason == "ErrImagePull") && ctx.Pod != nil {
		hasSecrets := len(ctx.Pod.Spec.ImagePullSecrets) > 0

		// Try to match well-known registry error patterns first.
		if m := imagePullMsgHint(ctx.Container.Msg, hasSecrets); m != "" {
			hint = hint + "; " + m
		} else if !hasSecrets {
			// No specific pattern — check if registry likely needs auth.
			for _, c := range ctx.Pod.Spec.Containers {
				if c.Name == ctx.Container.Container.Name && needsRegistryAuth(c.Image) {
					hint = hint + "; this image is from a registry that typically requires " +
						"authentication — add imagePullSecrets to the pod spec"
					break
				}
			}
		} else {
			hint = hint + "; imagePullSecrets is configured — check the image name/tag or secret validity"
		}
	}

	return hint
}

func buildProbeHint(reason string, spec *corev1.Container) string {
	var probe *corev1.Probe
	switch reason {
	case "LivenessProbeFailed":
		probe = spec.LivenessProbe
	case "ReadinessProbeFailed":
		probe = spec.ReadinessProbe
	case "StartupProbeFailed":
		probe = spec.StartupProbe
	}
	if probe == nil {
		return enricher.HintForReason(reason)
	}

	detail := reason
	if probe.HTTPGet != nil {
		detail = fmt.Sprintf("%s (HTTP GET http://%s%s:%d%s)", reason, spec.Name, probe.HTTPGet.Host, probe.HTTPGet.Port.IntValue(), probe.HTTPGet.Path)
	} else if probe.TCPSocket != nil {
		detail = fmt.Sprintf("%s (TCP check :%d)", reason, probe.TCPSocket.Port.IntValue())
	} else if probe.Exec != nil {
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
	case "LivenessProbeFailed":
		return "liveness"
	case "ReadinessProbeFailed":
		return "readiness"
	case "StartupProbeFailed":
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

// imagePullMsgHint returns a targeted fix suggestion when the image-pull
// error message matches a well-known pattern such as rate limiting or
// authentication failure.  Returns "" when no pattern matches.
func imagePullMsgHint(msg string, hasSecrets bool) string {
	msg = strings.ToLower(msg)

	switch {
	case strings.Contains(msg, "toomanyrequests") || strings.Contains(msg, "rate limit"):
		return "Docker Hub rate limit exceeded — add imagePullSecrets for authenticated pulls or configure a mirror registry"
	case strings.Contains(msg, "pull qps"):
		return "Kubelet image pull QPS limit exceeded — consider increasing registryPullQPS in kubelet config or reducing concurrent pod starts"
	case strings.Contains(msg, "authentication required") || strings.Contains(msg, "unauthorized") || strings.Contains(msg, "denied") || strings.Contains(msg, "no pull access"):
		return "Registry authentication failed — check imagePullSecrets validity"
	case strings.Contains(msg, "not found") || strings.Contains(msg, "manifest unknown") || strings.Contains(msg, "does not exist"):
		if hasSecrets {
			return "Image not found — check the image name/tag, or the image may not exist in this registry"
		}
		return "Image not found — check the image name/tag"
	case strings.Contains(msg, "context deadline exceeded") || strings.Contains(msg, "i/o timeout"):
		return "Registry connection timed out — check network connectivity to the registry and DNS resolution"
	case strings.Contains(msg, "connection refused") || strings.Contains(msg, "connection reset"):
		return "Registry connection refused — check that the registry is running and not blocked by a firewall"
	case strings.Contains(msg, "no route to host") || strings.Contains(msg, "network is unreachable"):
		return "No network route to registry — check firewall rules and network connectivity"
	case strings.Contains(msg, "no such host") || strings.Contains(msg, "dial tcp"):
		return "Registry unreachable — check cluster network connectivity and DNS"
	case strings.Contains(msg, "tls") || strings.Contains(msg, "certificate"):
		return "Registry TLS error — check registry certificate or configure insecure-registries"
	}
	return ""
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

// isPodTerminatingOrDisrupted returns true when the pod is in a terminal or
// terminating state where restart-count alerts should be suppressed
// (eviction, deletion, disruption target). Matches the same conditions as
// the DisruptionFilter to avoid firing HighRestartCount for intentionally
// terminated pods.
func isPodTerminatingOrDisrupted(pod *corev1.Pod) bool {
	if pod == nil {
		return false
	}
	if pod.DeletionTimestamp != nil {
		return true
	}
	if pod.Status.Phase == corev1.PodFailed && pod.Status.Reason == "Evicted" {
		return true
	}
	for _, c := range pod.Status.Conditions {
		if c.Type == "DisruptionTarget" {
			return true
		}
	}
	return false
}

func (h *handler) emitHighRestartAlert(ctx *filter.Context, container *corev1.ContainerStatus) {
	owner := correlation.ResolveOwnerName(ctx.Pod, h.rsLister, h.dsLister, h.ssLister)
	if owner == "" {
		return
	}

	lastReason, lastEC := lastTermInfo(container)

	h.signalEvent(&event.Signal{
		Resource:     "pod",
		PodName:      ctx.Pod.Name,
		Container:    container.Name,
		Namespace:    ctx.Pod.Namespace,
		NodeName:     ctx.Pod.Spec.NodeName,
		Reason:       "HighRestartCount",
		Labels:       ctx.Pod.Labels,
		RestartCount: container.RestartCount,
		Hint: fmt.Sprintf("container restarted %d times (last exit: %s, code %d)",
			container.RestartCount, lastReason, lastEC),
		Owner: owner,
		ContainerState: &model.ContainerState{
			RestartCount: container.RestartCount,
			Reason:       lastReason,
			ExitCode:     lastEC,
		},
	})
}
