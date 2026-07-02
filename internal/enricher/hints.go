package enricher

import "fmt"

var defaultHints = map[string]string{
	"OOMKilled":                "Memory pressure — consider increasing memory limits",
	"ImagePullBackOff":         "Registry or authentication issue — check image name and pull secret",
	"ErrImagePull":             "Registry or authentication issue — check image name and pull secret",
	"CrashLoopBackOff":             "Application crash — check logs for startup errors",
	"Error":                        "Container exited with error — check logs",
	"CreateContainerConfigError":   "Container configuration error — missing ConfigMap, Secret, or invalid volume mount",
	"ContainerStatusUnknown":       "Kubelet lost track of container state — node may be under resource pressure",
	"ImageInspectError":            "Kubernetes could not inspect the container image — check image format and registry accessibility",
	"InvalidImageName":             "Invalid container image name format — check image name syntax (e.g., 'repo/image:tag')",
	"RegistryUnavailable":          "Container image registry is unreachable — check registry availability and network connectivity",
	"NodeAffinity":                 "Pod node affinity rules prevent scheduling — check nodeSelector and affinity constraints",
	"DeadlineExceeded":             "Operation deadline exceeded — the container runtime or image pull timed out",
	"NodeNotReady":                 "Node not ready — check kubelet and node resources",
	"Unschedulable":            "No available node — check cluster capacity and resource requests",
	"InitContainerError":       "Init container failed — check init container logs",
	"BackOff":                  "Container crash — kubelet backing off before next restart",
	"ContainerCannotRun":       "Container runtime could not start the container — check entrypoint and binary architecture",
	"CreateContainerError":     "Container runtime failed to create the container — check volume mounts and cgroup configuration",
	"PostStartHookError":       "PostStart lifecycle hook failed — check container configuration",
	"PreStopHookError":         "PreStop lifecycle hook failed — check container configuration",
	"ProbeError":               "Probe execution failed — check probe command, port, or endpoint",
	"StartupProbeFailed":       "Startup probe failing — application is not starting within probe period",
	"ReadinessProbeFailed":     "Readiness probe failing — application is not ready to serve traffic",
	"LivenessProbeFailed":      "Liveness probe failing — application may be deadlocked or hung",
	"MemoryPressure":           "Node under memory pressure — consider reducing pod replicas or adding nodes",
	"DiskPressure":             "Node under disk pressure — free up disk space or add storage",
	"PIDPressure":              "Node under PID pressure — too many processes running",
	"NetworkUnavailable":       "Node network not available — check network plugin and connectivity",
	"ProgressDeadlineExceeded": "Rollout stuck — check pod template, resource limits, and deployment strategy",
	"JobFailed":                "Job failed — check job logs and exit code",
	"JobSuspended":             "Job suspended — check suspension request or cronjob configuration",
	"PodPending":               "Pod stuck in Pending — check scheduler, resources, and persistent volumes",
	"DaemonSetUnavailable":     "DaemonSet has unavailable pods — check node capacity and pod status",
	"CronJobSuspended":         "CronJob is suspended — check suspension request or schedule configuration",
	"CronJobNotScheduled":      "CronJob has not been scheduled recently — check schedule expression and job history",
}

var exitCodeHints = map[int32]string{
	1:   "General error — check application logs for details",
	2:   "Misuse of shell builtins — check command and arguments",
	126: "Command cannot execute — check file permissions on binary",
	127: "Command not found — check PATH or container image includes the binary",
	130: "Terminated by Ctrl+C (SIGINT)",
	137: "Out of memory (SIGKILL) — container exceeded memory limit",
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
