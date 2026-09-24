package incident

import (
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
)

type groupEntry struct {
	key       model.IncidentKey
	namespace string
	owner     string
	reason    string
	kind      string // "pod", "node", "deployment", etc.

	podName       string
	containerName string
	image         string
	nodeName      string

	// prevNotifiedSig is the member's notification signature from before
	// buffering overwrote it. A group of one is emitted as the member itself,
	// which needs the real value to tell a first alert from an update.
	prevNotifiedSig string
}

// ownerWindow records which owners have failed the same way in one namespace
// during the current grouping window. Entries remain buffered until the
// window expires so a namespace-wide failure can produce one message from the
// complete first wave instead of an individual alert followed by a group.
type ownerWindow struct {
	firstSeen time.Time
	owners    map[string]bool
	// announced is retained for persisted-state compatibility. New windows do
	// not populate it; restored older windows are still safe to read.
	announced map[string]model.IncidentKey
}

type pendingGroup struct {
	firstSeen     time.Time
	entries       []groupEntry
	overflowCount int
}

// groupFlushState records the last notification for a group key so repeated
// flushes of the same group re-notify on a stable key (update-not-create)
// and are throttled by a cooldown instead of flooding notifications.
type groupFlushState struct {
	notified       bool
	lastNotifiedAt time.Time
	// firstSeen is when the group first became active, carried across
	// re-flushes so reported durations reflect the real age of the problem.
	firstSeen       time.Time
	lastMemberCount int
}

type groupResolveTracker struct {
	groupIncKey model.IncidentKey
	members     map[model.IncidentKey]bool
	totalCount  int
	summary     string
	reason      string
	firstSeen   time.Time
	lastSeen    time.Time
	severity    model.Severity
}

const maxGroupEntries = 1000

func containsAny(s string, substrs ...string) bool {
	s = strings.ToLower(s)
	for _, sub := range substrs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func classifyImagePullScope(msg string) string {
	switch {
	case containsAny(msg, "toomanyrequests", "rate limit"):
		return "rate_limit"
	case containsAny(msg, "pull qps"):
		return "pull_qps"
	case containsAny(msg, "authentication required", "unauthorized",
		"denied", "no pull access"):
		return "auth"
	case containsAny(msg, "not found", "manifest unknown", "does not exist"):
		return "image_not_found"
	case containsAny(msg, "context deadline exceeded", "i/o timeout"):
		return "timeout"
	case containsAny(msg, "connection refused", "connection reset"):
		return "conn_refused"
	case containsAny(msg, "no route to host", "network is unreachable"):
		return "net_unreachable"
	case containsAny(msg, "no such host", "dial tcp"):
		return "dns"
	case containsAny(msg, "tls", "certificate"):
		return "tls"
	default:
		return ""
	}
}

func sharedMetricsAPIFailure(ev event.Event) bool {
	if ev.Facts.MetricFailure != "" {
		return ev.Facts.MetricFailure == "api_unavailable"
	}
	message := strings.ToLower(ev.Message + " " + ev.Hint)
	return containsAny(
		message,
		"server currently unable to handle the request",
		"no known available metric versions found",
		"metrics api is unavailable",
		"the metrics api is unavailable",
		"service unavailable",
	)
}

// ownerGroupKey is the owner-scoped grouping key: every incident with the
// same reason, in the same namespace, under the same workload shares it.
func ownerGroupKey(reason, namespace, owner string) string {
	return groupKeyCodec.EncodeOwner(reason, namespace, owner)
}

func encodeScopedGroupKey(reason, kind, value string) string {
	return groupKeyCodec.EncodeScope(reason, kind, value)
}

func encodeImageGroupKey(reason, image, namespace string) string {
	return groupKeyCodec.EncodeImage(reason, image, namespace)
}

func groupNamespace(kind, value string) string {
	if kind == "ns" || kind == "cp" {
		return value
	}
	return ""
}

// groupPlan is the decision half of smart grouping: which group an incident
// belongs to, and the member entry that will stand for it.
//
// Grouping used to decide and mutate in one pass -- the window index, the
// buffer, the incident's own notification fields and the metric counter were
// all written while the key was still being worked out, so there was no point
// at which the decision could be inspected on its own. Splitting the two
// leaves a pure function that answers "what group is this?" and a short
// applier that performs the writes.
type groupPlan struct {
	key   string
	entry groupEntry
}

// planGroupEntry computes the group an incident belongs to. Pure: it reads
// only its arguments and writes nothing.
func planGroupEntry(
	inc *model.Incident,
	ev event.Event,
	owner string,
) groupPlan {
	r := normalizeReason(ev.Reason)
	return groupPlan{
		key: computeGroupKey(r, ev, owner),
		entry: groupEntry{
			key:             inc.Key,
			prevNotifiedSig: inc.NotifiedSig,
			namespace:       ev.Namespace,
			owner:           owner,
			reason:          r,
			kind:            ev.Resource,
			podName:         ev.PodName,
			containerName:   ev.ContainerName,
			image:           ev.Image,
			nodeName:        ev.NodeName,
		},
	}
}

// computeGroupKey maps a reason onto the structured scope its incidents
// should share. Application log content never participates in identity.
func computeGroupKey(r string, ev event.Event, owner string) string {
	switch r {
	case constant.ReasonOOMKilled,
		constant.ReasonOOMRepeating,
		constant.ReasonCrashLoopHighFreq,
		constant.ReasonHighRestartCount,
		constant.ReasonInitContainerError,
		constant.ReasonContainerCannotRun,
		constant.ReasonCreateContainerError,
		constant.ReasonDeadlineExceeded,
		constant.ReasonStartupProbeFailed,
		constant.ReasonLivenessProbeFailed,
		constant.ReasonReadinessProbeFailed,
		constant.ReasonProbeError,
		constant.ReasonPostStartHookError,
		constant.ReasonPreStopHookError,
		constant.ReasonNodeAffinity,
		constant.ReasonProgressDeadlineExceeded,
		constant.ReasonDeploymentUnavailable,
		constant.ReasonDaemonSetUnavailable,
		constant.ReasonStsUnavailable,
		constant.ReasonPdbViolation,
		constant.ReasonHPAMaxedOut,
		constant.ReasonHPAScalingError,
		constant.ReasonJobFailed,
		constant.ReasonJobSuspended,
		constant.ReasonCronJobSuspended,
		constant.ReasonCronJobNotScheduled,
		constant.ReasonVolumeUsageHigh,
		constant.ReasonPreExistingAtStartup:
		return ownerGroupKey(r, ev.Namespace, owner)

	case constant.ReasonFailedGetResourceMetric:
		if sharedMetricsAPIFailure(ev) {
			return encodeScopedGroupKey(r, "global", "metrics-api")
		}
		return ownerGroupKey(r, ev.Namespace, owner)

	case constant.ReasonCrashLoopBackOff,
		constant.ReasonBackOff,
		constant.ReasonError:
		return ownerGroupKey(r, ev.Namespace, owner)

	case constant.ReasonImagePullBackOff, constant.ReasonErrImagePull:
		scope := classifyImagePullScope(ev.Message)
		switch scope {
		case "rate_limit", "pull_qps", "timeout", "conn_refused",
			"net_unreachable", "dns", "tls":
			return encodeScopedGroupKey(r, "global", scope)
		case "auth":
			return encodeScopedGroupKey(r, "ns", ev.Namespace)
		case "image_not_found":
			return encodeImageGroupKey(r, ev.Image, ev.Namespace)
		default:
			return encodeImageGroupKey(r, ev.Image, ev.Namespace)
		}

	case constant.ReasonImageInspectError, constant.ReasonInvalidImageName:
		return encodeImageGroupKey(r, ev.Image, ev.Namespace)

	case constant.ReasonNodeNotReady,
		constant.ReasonMemoryPressure,
		constant.ReasonDiskPressure,
		constant.ReasonPIDPressure,
		constant.ReasonNetworkUnavailable,
		constant.ReasonContainerStatusKnown,
		constant.ReasonEvicted,
		constant.ReasonPreempting,
		constant.ReasonNodeResourceHigh,
		constant.ReasonNodeResourceCritical:
		return encodeScopedGroupKey(r, "node", ev.NodeName)

	case constant.ReasonServiceNoEndpoints:
		return encodeScopedGroupKey(r, "svc", ev.Namespace+"/"+ev.PodName)

	case constant.ReasonControlPlaneComponentFailure:
		return encodeScopedGroupKey(r, "cp", ev.Namespace)

	case constant.ReasonCreateConfigError,
		constant.ReasonUnschedulable,
		constant.ReasonPodPending,
		constant.ReasonSchedulingGated,
		constant.ReasonRegistryUnavailable,
		constant.ReasonTLSCertExpired,
		constant.ReasonTLSCertExpiringSoon:
		return encodeScopedGroupKey(r, "ns", ev.Namespace)

	default:
		return ownerGroupKey(r, ev.Namespace, owner)
	}
}
