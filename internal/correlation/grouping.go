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
	MassFailureHook func()
	BaselineTTL     time.Duration
	// OwnerBaselineTTL bounds owner-level baseline entries, which are seeded
	// with an empty pod name and cover every Pod of that owner.
	//
	// They shared BaselineTTL (24h). Nothing else expires them: only an
	// incident's resolve clears a baseline entry, and no incident exists
	// because the entry suppressed it. A Deployment mid-rollout when kwatch
	// started was therefore silenced for a day -- including when it recovered
	// and broke again for real. This only has to outlive the startup burst.
	OwnerBaselineTTL           time.Duration
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
	// SubjectPresent reports whether the object an incident is about still
	// exists, and whether the caller can answer for that kind at all.
	//
	// Staleness alone is a poor resolve signal. A Deployment wedged on a
	// failed rollout stops producing events once the last replica gives up,
	// and the engine then closed the incident as if the rollout had
	// succeeded. Asking whether the object is still there separates "gone,
	// so genuinely finished" from "still broken, just quiet". Nil keeps the
	// old staleness-only behaviour.
	SubjectPresent func(resource, namespace, name string) (exists, known bool)
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

// ownerGroupKey is the owner-scoped grouping key: every incident with the
// same reason, in the same namespace, under the same workload shares it.
//
// It was spelled out by hand in five places -- three arms of computeGroupKey,
// the fan-out scope check, and the group-folding loop. The same string built
// five ways is the shape of bug that takes an afternoon to find, because a
// mismatch does not fail: the lookup simply misses and the group silently
// does not fold.
func ownerGroupKey(reason, namespace, owner string) string {
	return reason + "|" + namespace + "|" + owner
}

// groupSignature is the log-derived identity used when the same crash message
// appears across otherwise unrelated workloads. Only the log-bearing reasons
// have one, and it is computed once per incident: extracting a signature walks
// the whole log tail, and grouping used to do it twice for every event.
func groupSignature(r, logs string) string {
	switch r {
	case constant.ReasonCrashLoopBackOff,
		constant.ReasonBackOff,
		constant.ReasonError:
		return enricher.SignatureHint(logs)
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
	sig := groupSignature(r, ev.Logs)
	return groupPlan{
		key: computeGroupKey(r, ev, owner, sig),
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
			logSignature:    sig,
		},
	}
}

// computeGroupKey maps a reason onto the scope its incidents should share.
// sig is the precomputed log signature from groupSignature; it is only
// consulted for the log-bearing reasons.
func computeGroupKey(r string, ev event.Event, owner, sig string) string {
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

	case constant.ReasonCrashLoopBackOff,
		constant.ReasonBackOff,
		constant.ReasonError:
		if sig != "" {
			return r + "|sig|" + sig
		}
		return ownerGroupKey(r, ev.Namespace, owner)

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
		return ownerGroupKey(r, ev.Namespace, owner)
	}
}
