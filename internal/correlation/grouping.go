package correlation

import (
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/enricher"
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
	logSignature  string

	// prevNotifiedSig is the member's notification signature from before
	// buffering overwrote it. A group of one is emitted as the member itself,
	// which needs the real value to tell a first alert from an update.
	prevNotifiedSig string
}

// ownerWindow records which owners have failed the same way in one namespace
// during the current grouping window, and which of them were announced
// immediately rather than buffered.
//
// An owner-scoped group ("reason|namespace|owner") can only ever hold one
// incident, because the incident key is owner-scoped too — so buffering the
// first owner never groups anything; it only delays the most common alert by a
// whole window. The first owner is therefore announced at once. Buffering
// starts with the second owner, which is the earliest moment a namespace-wide
// fan-out can be told apart from an isolated failure.
type ownerWindow struct {
	firstSeen time.Time
	owners    map[string]bool
	// owner → incident key announced immediately
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
	firstSeen time.Time
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

type Config struct {
	Window            time.Duration
	LifecycleInterval time.Duration
	Enricher          enricher.Enricher
	LifecycleHook     func(inc *model.Incident, action model.IncidentAction)
	// called during lifecycle tick; reports mass failures
	MassFailureHook            func()
	BaselineTTL                time.Duration
	Baseline                   map[string]map[string]int64
	OnBaselineChange           func(baseline map[string]map[string]int64)
	EscalationEnabled          bool
	EscalationTiers            []int
	InhibitNodeSuppressesPods  bool
	MaxBaseline                int
	RenotifyIntervalBySeverity map[string]time.Duration
	RenotifyMaxPerIncident     int
	ResolveHoldDown            time.Duration
	Runbooks                   map[string]string
	SmartGroupingWindow        time.Duration
	// DependenciesOf resolves the shared dependencies an incident touches, so
	// the engine can suppress symptoms already covered by a mass-failure
	// alert. Supplied by the app, which owns the resource graph. Nil disables
	// mass-failure suppression.
	DependenciesOf func(*model.Incident) []string
	// NamespaceFanOutThreshold is how many distinct owners must fail the same
	// way, in one namespace, inside one grouping window before their per-owner
	// groups are collapsed into a single namespace-level notification. Zero
	// disables the collapse.
	NamespaceFanOutThreshold int
}

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
		return r + "|" + ev.Namespace + "|" + owner

	case constant.ReasonCrashLoopBackOff,
		constant.ReasonBackOff,
		constant.ReasonError:
		if sig := enricher.SignatureHint(ev.Logs); sig != "" {
			return r + "|sig|" + sig
		}
		return r + "|" + ev.Namespace + "|" + owner

	case constant.ReasonImagePullBackOff, constant.ReasonErrImagePull:
		scope := classifyImagePullScope(ev.Message)
		switch scope {
		case "rate_limit", "pull_qps", "timeout", "conn_refused",
			"net_unreachable", "dns", "tls":
			return r + "|global|" + scope
		case "auth":
			return r + "|ns|" + ev.Namespace
		case "image_not_found":
			return r + "|img|" + ev.Image + "|ns|" + ev.Namespace
		default:
			return r + "|img|" + ev.Image + "|ns|" + ev.Namespace
		}

	case constant.ReasonImageInspectError, constant.ReasonInvalidImageName:
		return r + "|img|" + ev.Image + "|ns|" + ev.Namespace

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
		return r + "|node|" + ev.NodeName

	case constant.ReasonServiceNoEndpoints:
		return r + "|svc|" + ev.Namespace + "/" + ev.PodName

	case constant.ReasonControlPlaneComponentFailure:
		return r + "|cp|" + ev.Namespace

	case constant.ReasonCreateConfigError,
		constant.ReasonUnschedulable,
		constant.ReasonPodPending,
		constant.ReasonSchedulingGated,
		constant.ReasonRegistryUnavailable,
		constant.ReasonTLSCertExpired,
		constant.ReasonTLSCertExpiringSoon:
		return r + "|ns|" + ev.Namespace

	default:
		return r + "|" + ev.Namespace + "|" + owner
	}
}
