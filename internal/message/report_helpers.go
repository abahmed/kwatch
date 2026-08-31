package message

import (
	"fmt"
	"time"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/model"
)

// --- Helpers ---

// registryHint is the kubelet's own message for an image-pull failure, e.g.
// `Back-off pulling image "ghcr.io/x/y:v2"` — the most specific thing we know.
func registryHint(inc *model.Incident) string {
	if inc.LastContainerState == nil {
		return ""
	}
	return inc.LastContainerState.Msg
}

func durationStr(first, last time.Time) string {
	d := last.Sub(first).Round(time.Minute)
	if d < time.Minute {
		return "< 1m"
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
}

func actionString(a model.IncidentAction) string {
	switch a {
	case model.ActionCreate:
		return "create"
	case model.ActionUpdate:
		return "update"
	case model.ActionResolved:
		return "resolved"
	default:
		return "unknown"
	}
}

func actionEmoji(action model.IncidentAction, severity model.Severity) string {
	switch action {
	case model.ActionResolved:
		return "✅"
	case model.ActionUpdate:
		return "🔄"
	default:
		// Red is reserved for critical. A normal alert that shows the same
		// colour as a critical one teaches readers to ignore the colour.
		switch severity {
		case model.SeverityCritical:
			return "🔴"
		case model.SeverityHigh:
			return "🟠"
		case model.SeverityWarning, model.SeverityMedium:
			return "🟡"
		default:
			return "🔵"
		}
	}
}

func probeTypeFromReason(reason string) string {
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

// reasonLabels maps a Kubernetes reason code to the phrase a person would use
// for it. The code stays visible on the headline for searching; this is what
// the reader parses first.
var reasonLabels = map[string]string{
	constant.ReasonOOMKilled:            "Out of memory",
	constant.ReasonCrashLoopBackOff:     "Container keeps crashing",
	constant.ReasonImagePullBackOff:     "Failed to download image",
	constant.ReasonErrImagePull:         "Failed to download image",
	constant.ReasonLivenessProbeFailed:  "Health check failed",
	constant.ReasonReadinessProbeFailed: "Not ready for traffic",
	constant.ReasonStartupProbeFailed:   "Startup check failed",
	constant.ReasonUnschedulable:        "No capacity available",
	"Pending":                           "Waiting for resources",
	constant.ReasonNodeNotReady:         "Node is not ready",
	constant.ReasonBackOff:              "Backing off after crash",
	constant.ReasonError:                "Container error",
	constant.ReasonHighRestartCount:     "Frequent restarts",
	constant.ReasonInitContainerError:   "Init container failed",
	constant.ReasonOOMRepeating:         "Repeated out of memory",
	constant.ReasonEvicted:              "Pod was evicted",
	constant.ReasonContainersNotReady:   "Pod not ready",

	// workloads
	constant.ReasonDeploymentUnavailable: "Deployment below desired replicas",

	constant.ReasonDaemonSetUnavailable: "DaemonSet below desired replicas",

	constant.ReasonStsUnavailable: "StatefulSet below desired replicas",

	constant.ReasonProgressDeadlineExceeded: "Rollout stuck",

	constant.ReasonHPAMaxedOut: "Autoscaler pinned at max replicas",

	constant.ReasonHPAScalingError:     "Autoscaler cannot scale",
	constant.ReasonJobFailed:           "Job failed",
	constant.ReasonJobSuspended:        "Job suspended",
	constant.ReasonCronJobSuspended:    "CronJob suspended",
	constant.ReasonCronJobNotScheduled: "CronJob missed its schedule",
	constant.ReasonPdbViolation:        "Disruption budget violated",

	// networking
	constant.ReasonServiceNoEndpoints:     "Service has no healthy backends",
	constant.ReasonIngressBackendNotFound: "Ingress points at missing service",
	constant.ReasonWebhookBackendNotFound: "Admission webhook has no backend",
	constant.ReasonTLSCertExpired:         "TLS certificate expired",
	constant.ReasonTLSCertExpiringSoon:    "TLS certificate expiring soon",

	// cluster
	constant.ReasonControlPlaneComponentFailure:     "Control-plane component failing",
	constant.ReasonAPIServerUnavailable:             "Kubernetes API server unavailable",
	constant.ReasonAPIServerLatency:                 "Kubernetes API server is slow",
	constant.ReasonSchedulerUnavailable:             "Kubernetes scheduler health check failed",
	constant.ReasonControllerManagerUnavailable:     "Kubernetes controller-manager health check failed",
	constant.ReasonEtcdUnavailable:                  "etcd health check failed",
	constant.ReasonCoreDNSUnavailable:               "CoreDNS DNS resolution failed",
	constant.ReasonActiveProbeLatency:               "Application probe latency is high",
	constant.ReasonVolumeAttachmentFailure:          "Volume attachment failed",
	constant.ReasonVolumeSnapshotFailure:            "Volume snapshot failed",
	constant.ReasonMutatingAdmissionPolicyInvalid:   "Mutating admission policy is invalid",
	constant.ReasonCertificateSigningRequestFailure: "Certificate request failed or is not being signed",
	constant.ReasonAPIPriorityAndFairnessFailure:    "API Priority and Fairness configuration is invalid",

	constant.ReasonVolumeUsageHigh:      "Volume running out of space",
	constant.ReasonNodeResourceHigh:     "Node overcommitted",
	constant.ReasonNodeResourceCritical: "Node overcommitted",
	constant.ReasonMemoryPressure:       "Node under memory pressure",
	constant.ReasonNodeMemoryPressure:   "Node under memory pressure",
	constant.ReasonDiskPressure:         "Node under disk pressure",
	constant.ReasonPIDPressure:          "Node out of process IDs",
	constant.ReasonNetworkUnavailable:   "Node network unavailable",
	constant.ReasonContainerCannotRun:   "Container cannot start",
	constant.ReasonCreateContainerError: "Container cannot start",
	constant.ReasonCreateConfigError:    "Missing ConfigMap or Secret",
	constant.ReasonDeadlineExceeded:     "Ran past its deadline",
	constant.ReasonImageInspectError:    "Invalid image reference",
	constant.ReasonInvalidImageName:     "Invalid image reference",
	constant.ReasonSandboxError:         "Pod sandbox failed",
	constant.ReasonPreempting:           "Pod preempted",

	constant.ReasonPreExistingAtStartup: "Already broken when kwatch started",
}

// reasonLabel returns the human phrase for a reason, or the reason itself
// when there is none.
func reasonLabel(reason string) string {
	if label, ok := reasonLabels[reason]; ok {
		return label
	}
	return reason
}

// ageOf renders how long ago t was, coarsely — "40s", "3m", "2h". Empty for
// the zero time.
func ageOf(t, now time.Time) string {
	if t.IsZero() {
		return ""
	}
	d := now.Sub(t)
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	default:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
}
