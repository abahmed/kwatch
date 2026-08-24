package correlation

import (
	"fmt"
	"hash/crc32"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/enricher"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/metrics"
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
	Window                     time.Duration
	LifecycleInterval          time.Duration
	Enricher                   enricher.Enricher
	LifecycleHook              func(inc *model.Incident, action model.IncidentAction)
	MassFailureHook            func() // called during lifecycle tick; reports mass failures
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
	case constant.ReasonOOMKilled, constant.ReasonOOMRepeating, constant.ReasonCrashLoopHighFreq, constant.ReasonHighRestartCount,
		constant.ReasonInitContainerError,
		constant.ReasonContainerCannotRun, constant.ReasonCreateContainerError,
		constant.ReasonDeadlineExceeded,
		constant.ReasonStartupProbeFailed, constant.ReasonLivenessProbeFailed,
		constant.ReasonReadinessProbeFailed, constant.ReasonProbeError,
		constant.ReasonPostStartHookError, constant.ReasonPreStopHookError,
		constant.ReasonNodeAffinity,
		constant.ReasonProgressDeadlineExceeded, constant.ReasonDeploymentUnavailable,
		constant.ReasonDaemonSetUnavailable,
		constant.ReasonStsUnavailable,
		constant.ReasonPdbViolation,
		constant.ReasonHPAMaxedOut, constant.ReasonHPAScalingError,
		constant.ReasonJobFailed, constant.ReasonJobSuspended,
		constant.ReasonCronJobSuspended, constant.ReasonCronJobNotScheduled,
		constant.ReasonVolumeUsageHigh,
		constant.ReasonPreExistingAtStartup:
		return r + "|" + ev.Namespace + "|" + owner

	case constant.ReasonCrashLoopBackOff, constant.ReasonBackOff, constant.ReasonError:
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

	case constant.ReasonNodeNotReady, constant.ReasonMemoryPressure, constant.ReasonDiskPressure,
		constant.ReasonPIDPressure, constant.ReasonNetworkUnavailable,
		constant.ReasonContainerStatusKnown, constant.ReasonEvicted, constant.ReasonPreempting,
		constant.ReasonNodeResourceHigh, constant.ReasonNodeResourceCritical:
		return r + "|node|" + ev.NodeName

	case constant.ReasonServiceNoEndpoints:
		return r + "|svc|" + ev.Namespace + "/" + ev.PodName

	case constant.ReasonControlPlaneComponentFailure:
		return r + "|cp|" + ev.Namespace

	case constant.ReasonCreateConfigError, constant.ReasonUnschedulable, constant.ReasonPodPending,
		constant.ReasonSchedulingGated,
		constant.ReasonRegistryUnavailable,
		constant.ReasonTLSCertExpired, constant.ReasonTLSCertExpiringSoon:
		return r + "|ns|" + ev.Namespace

	default:
		return r + "|" + ev.Namespace + "|" + owner
	}
}

// groupRenotifyCooldown returns the minimum interval between notifications
// for the same smart group. Re-flushes within the interval silently refresh
// the group's state without notifying, so a busy group can't spam updates on
// every flush window.
func (e *Engine) groupRenotifyCooldown() time.Duration {
	w := e.config.SmartGroupingWindow
	cd := 4 * w
	if cd < 5*time.Minute {
		cd = 5 * time.Minute
	}
	if cd > 30*time.Minute {
		cd = 30 * time.Minute
	}
	return cd
}

// groupedKeys returns the set of incident keys currently absorbed into a
// smart group (buffered in a pending group or tracked as a flushed group
// member). Renotify skips these — the group notification is the single
// re-notification channel for grouped incidents, so per-member renotify
// would duplicate the group alert. Caller must hold e.mu.
func (e *Engine) groupedKeys() map[model.IncidentKey]bool {
	keys := make(map[model.IncidentKey]bool)
	for _, pg := range e.groupBuffers {
		for _, ge := range pg.entries {
			keys[ge.key] = true
		}
	}
	for _, tracker := range e.groupResolveTrackers {
		for k := range tracker.members {
			keys[k] = true
		}
	}
	return keys
}

// rekeyGroupReferences moves any smart-group membership for an incident from
// oldKey to newKey. Used when the crash-loop fold re-keys an incident so the
// group keeps tracking it (otherwise the group would wait forever on a member
// that no longer exists). Caller must hold e.mu.
func (e *Engine) rekeyGroupReferences(oldKey, newKey model.IncidentKey) {
	for _, pg := range e.groupBuffers {
		for i := range pg.entries {
			if pg.entries[i].key == oldKey {
				pg.entries[i].key = newKey
			}
		}
	}
	for _, tracker := range e.groupResolveTrackers {
		if _, ok := tracker.members[oldKey]; ok {
			delete(tracker.members, oldKey)
			tracker.members[newKey] = false
		}
	}
}

// Caller must hold e.mu.
// tryGroupIncident attempts to add an event to the smart grouping buffer.
// Returns true if the incident was grouped (caller should return ActionSkip).
func (e *Engine) tryGroupIncident(inc *model.Incident, ev event.Event, owner string, now time.Time) bool {
	if e.config.SmartGroupingWindow <= 0 || inc.NotifiedSig != "" {
		return false
	}
	r := normalizeReason(ev.Reason)
	gk := computeGroupKey(r, ev, owner)
	pg, ok := e.groupBuffers[gk]
	if !ok {
		pg = &pendingGroup{firstSeen: now}
		e.groupBuffers[gk] = pg
	}
	sig := ""
	if r == constant.ReasonCrashLoopBackOff || r == constant.ReasonBackOff || r == constant.ReasonError {
		sig = enricher.SignatureHint(ev.Logs)
	}
	entry := groupEntry{
		key:           inc.Key,
		namespace:     ev.Namespace,
		owner:         owner,
		reason:        r,
		kind:          ev.Resource,
		podName:       ev.PodName,
		containerName: ev.ContainerName,
		image:         ev.Image,
		nodeName:      ev.NodeName,
		logSignature:  sig,
	}
	pg.entries = append(pg.entries, entry)
	if len(pg.entries) > maxGroupEntries {
		pg.entries = pg.entries[1:]
		pg.overflowCount++
	}
	inc.NotifiedSig = notifSig(inc)
	inc.LastNotifiedAt = now
	metrics.Default.IncidentsGrouped.Add(1)
	return true
}

// Caller must hold e.mu.
// buildGroupSummary produces a human-readable summary for a smart grouping

// groupMemberResolved marks key as resolved within its group tracker and
// returns the group's resolved notification once every member has resolved.
// Caller must hold e.mu. tracked reports whether key belonged to a group.
// Members removed from state outside MarkResolved (orphan folding, resource
// resolution) must go through here so the group isn't left waiting forever
// on a member that no longer exists.
func (e *Engine) groupMemberResolved(key model.IncidentKey) (groupInc *model.Incident, action model.IncidentAction, tracked bool) {
	for gk, tracker := range e.groupResolveTrackers {
		if _, ok := tracker.members[key]; ok {
			tracker.members[key] = true
			allResolved := true
			for _, resolved := range tracker.members {
				if !resolved {
					allResolved = false
					break
				}
			}
			if !allResolved {
				return nil, model.ActionSkip, true
			}
			delete(e.groupResolveTrackers, gk)
			// Reset the flush state so a genuinely new occurrence of the
			// same group creates a fresh incident (a stable-key UPDATE
			// after RESOLVED would otherwise re-open a closed incident).
			delete(e.groupFlushStates, gk)
			groupInc := &model.Incident{
				ID:        fmt.Sprintf("%08x", crc32.ChecksumIEEE([]byte(tracker.groupIncKey))),
				Key:       tracker.groupIncKey,
				Reason:    tracker.reason,
				Name:      tracker.summary,
				Count:     tracker.totalCount,
				FirstSeen: tracker.firstSeen,
				LastSeen:  tracker.lastSeen,
				State:     model.StateResolved,
				Severity:  tracker.severity,
				Hint:      tracker.summary,
			}
			return groupInc, model.ActionResolved, true
		}
	}
	return nil, model.ActionSkip, false
}

// Caller must hold e.mu.
func (e *Engine) tryConsumeGroupResolve(key model.IncidentKey) (groupInc *model.Incident, action model.IncidentAction, tracked bool) {
	return e.groupMemberResolved(key)
}
