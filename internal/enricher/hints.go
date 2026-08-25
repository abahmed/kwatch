package enricher

import (
	"fmt"

	"github.com/abahmed/kwatch/internal/constant"
)

var defaultHints = map[string]string{
	constant.ReasonOOMKilled:                "Memory pressure — consider increasing memory limits",
	constant.ReasonImagePullBackOff:         "Registry or authentication issue — check image name and pull secret",
	constant.ReasonErrImagePull:             "Registry or authentication issue — check image name and pull secret",
	constant.ReasonCrashLoopBackOff:         "Application crash — check logs for startup errors",
	constant.ReasonError:                    "Container exited with error — check logs",
	constant.ReasonCreateConfigError:        "Container configuration error — missing ConfigMap, Secret, or invalid volume mount",
	constant.ReasonContainerStatusKnown:     "Kubelet lost track of container state — node may be under resource pressure",
	constant.ReasonImageInspectError:        "Kubernetes could not inspect the container image — check image format and registry accessibility",
	constant.ReasonInvalidImageName:         "Invalid container image name format — check image name syntax (e.g., 'repo/image:tag')",
	constant.ReasonRegistryUnavailable:      "Container image registry is unreachable — check registry availability and network connectivity",
	constant.ReasonNodeAffinity:             "Pod node affinity rules prevent scheduling — check nodeSelector and affinity constraints",
	constant.ReasonDeadlineExceeded:         "Operation deadline exceeded — the container runtime or image pull timed out",
	constant.ReasonNodeNotReady:             "Node not ready — check kubelet and node resources",
	constant.ReasonUnschedulable:            "No available node — check cluster capacity and resource requests",
	constant.ReasonInitContainerError:       "Init container failed — check init container logs",
	constant.ReasonBackOff:                  "Container crash — kubelet backing off before next restart",
	constant.ReasonContainerCannotRun:       "Container runtime could not start the container — check entrypoint and binary architecture",
	constant.ReasonCreateContainerError:     "Container runtime failed to create the container — check volume mounts and cgroup configuration",
	constant.ReasonPostStartHookError:       "PostStart lifecycle hook failed — check container configuration",
	constant.ReasonPreStopHookError:         "PreStop lifecycle hook failed — check container configuration",
	constant.ReasonProbeError:               "Probe execution failed — check probe command, port, or endpoint",
	constant.ReasonStartupProbeFailed:       "Startup probe failing — application is not starting within probe period",
	constant.ReasonReadinessProbeFailed:     "Readiness probe failing — application is not ready to serve traffic",
	constant.ReasonLivenessProbeFailed:      "Liveness probe failing — application may be deadlocked or hung",
	constant.ReasonMemoryPressure:           "Node under memory pressure — consider reducing pod replicas or adding nodes",
	constant.ReasonDiskPressure:             "Node under disk pressure — free up disk space or add storage",
	constant.ReasonPIDPressure:              "Node under PID pressure — too many processes running",
	constant.ReasonNetworkUnavailable:       "Node network not available — check network plugin and connectivity",
	constant.ReasonProgressDeadlineExceeded: "Rollout stuck — check pod template, resource limits, and deployment strategy",
	constant.ReasonJobFailed:                "Job failed — check job logs and exit code",
	constant.ReasonJobSuspended:             "Job suspended — check suspension request or cronjob configuration",
	constant.ReasonPodPending:               "Pod stuck in Pending — check scheduler, resources, and persistent volumes",
	constant.ReasonDaemonSetUnavailable:     "DaemonSet has unavailable pods — check node capacity and pod status",
	constant.ReasonCronJobSuspended:         "CronJob is suspended — check suspension request or schedule configuration",
	constant.ReasonCronJobNotScheduled:      "CronJob has not been scheduled recently — check schedule expression and job history",
	constant.ReasonOOMRepeating:             "Container repeatedly OOM-killed — potential memory leak; consider increasing memory limits or investigating memory growth",
	constant.ReasonStsUnavailable:           "StatefulSet has unavailable pods — check PVC, pod status, or rollout progress",
	constant.ReasonPdbViolation:             "PodDisruptionBudget is blocking voluntary disruptions — check pod health or reduce replica count",
	constant.ReasonNodeResourceHigh:         "Node resource overcommit is high — CPU or memory requests exceed allocatable by a significant margin",
	constant.ReasonNodeResourceCritical:     "Node resource overcommit is critical — CPU or memory requests far exceed allocatable; risk of resource starvation",
}

var exitCodeHints = map[int32]string{
	1:   "General error — check application logs for details",
	2:   "Misuse of shell builtins — check command and arguments",
	126: "Command cannot execute — check file permissions on binary",
	127: "Command not found — check PATH or container image includes the binary",
	130: "Terminated by Ctrl+C (SIGINT)",
	137: "Terminated by SIGKILL (exit 137) — may be OOM-killed, evicted, or killed by a liveness probe",
	139: "Segmentation fault (SIGSEGV) — null pointer or buffer overflow",
	143: "Graceful shutdown (SIGTERM)",
	255: "Exit status out of range — check entrypoint script",
}

func hintForReason(reason string) string {
	if h, ok := defaultHints[reason]; ok {
		return h
	}
	return ""
}

func HintForReason(reason string) string {
	return hintForReason(reason)
}

func hintForExitCode(code int32) string {
	if h, ok := exitCodeHints[code]; ok {
		return h
	}
	if code > 0 {
		return fmt.Sprintf("Non-zero exit code %d — check application logs", code)
	}
	return ""
}

func HintForExitCode(code int32) string {
	return hintForExitCode(code)
}

// combineHints appends a secondary hint to a primary hint when both are non-empty.
func combineHints(primary, secondary string) string {
	if primary == "" {
		return secondary
	}
	if secondary == "" {
		return primary
	}
	return primary + "; " + secondary
}

func CombineHints(primary, secondary string) string {
	return combineHints(primary, secondary)
}
