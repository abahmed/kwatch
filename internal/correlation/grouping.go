package correlation

import (
	"fmt"
	"hash/crc32"
	"sort"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/enricher"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/metrics"
	"github.com/abahmed/kwatch/internal/model"
)

type groupEntry struct {
	key       string
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

type groupResolveTracker struct {
	groupIncKey string
	members     map[string]bool
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

// Caller must hold e.mu.
// tryGroupIncident attempts to add an event to the smart grouping buffer.
// Returns true if the incident was grouped (caller should return ActionSkip).
func (e *Engine) tryGroupIncident(inc *model.Incident, ev event.Event, owner string, now time.Time) bool {
	if e.config.SmartGroupingWindow <= 0 || inc.NotifiedSig != "" {
		return false
	}
	r := normalizeReason(ev.Reason)
	gk := computeGroupKey(r, ev, owner)
	pg, ok := e.pendingGroups[gk]
	if !ok {
		pg = &pendingGroup{firstSeen: now}
		e.pendingGroups[gk] = pg
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
// flush using scope-adapted format.
func (e *Engine) buildGroupSummary(entries []groupEntry, firstSeen time.Time) string {
	if len(entries) == 0 {
		return ""
	}
	scope := detectGroupScope(entries)
	r := entries[0].reason
	timeAgo := ""
	if d := e.now().Sub(firstSeen).Round(time.Second); d > 0 {
		timeAgo = fmt.Sprintf(" — %s", timeAgoStr(d))
	}
	switch scope {
	case "node":
		return e.buildNodeSummary(r, entries, timeAgo)
	case "signature":
		return e.buildSignatureSummary(r, entries, timeAgo)
	case "image":
		return e.buildImageSummary(r, entries, timeAgo)
	case "owner":
		return e.buildOwnerSummary(r, entries, timeAgo)
	case "namespace":
		return e.buildNamespaceSummary(r, entries, timeAgo)
	default:
		return e.buildGenericSummary(r, entries, timeAgo)
	}
}

func timeAgoStr(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	return fmt.Sprintf("%dh", int(d.Hours()))
}

func pluralize(s string, n int) string {
	if n == 1 {
		return s
	}
	return s + "s"
}

func detectGroupScope(entries []groupEntry) string {
	r := entries[0].reason
	nodes := uniqueNonEmptyStr(entries, func(e groupEntry) string { return e.nodeName })
	if len(nodes) == 1 && isNodeLevelReason(r) {
		return "node"
	}
	sigs := uniqueNonEmptyStr(entries, func(e groupEntry) string { return e.logSignature })
	if len(sigs) == 1 {
		return "signature"
	}
	imgs := uniqueNonEmptyStr(entries, func(e groupEntry) string { return e.image })
	if len(imgs) == 1 {
		return "image"
	}
	owners := uniqueNonEmptyStr(entries, func(e groupEntry) string { return e.owner })
	if len(owners) == 1 {
		return "owner"
	}
	return "generic"
}

func uniqueNonEmptyStr[T any](entries []T, fn func(T) string) []string {
	m := make(map[string]bool)
	for _, e := range entries {
		if v := fn(e); v != "" {
			m[v] = true
		}
	}
	out := make([]string, 0, len(m))
	for v := range m {
		out = append(out, v)
	}
	return out
}

func (e *Engine) buildOwnerSummary(r string, entries []groupEntry, timeAgo string) string {
	byPod := make(map[string]int)
	for _, ge := range entries {
		byPod[ge.podName]++
	}
	podList := make([]string, 0, len(byPod))
	for p := range byPod {
		podList = append(podList, p)
	}
	sort.Strings(podList)
	owner := entries[0].namespace + "/" + entries[0].owner
	total := len(entries)
	return fmt.Sprintf("%s — %d %s in %s (total %d)%s",
		r, len(byPod), pluralize("pod", len(byPod)), owner, total, timeAgo)
}

func (e *Engine) buildNodeSummary(r string, entries []groupEntry, timeAgo string) string {
	node := entries[0].nodeName
	byPod := make(map[string]int)
	for _, ge := range entries {
		byPod[ge.podName]++
	}
	podCount := len(byPod)
	total := len(entries)
	return fmt.Sprintf("%s — node: %s, %d %s affected (total %d)%s",
		r, node, podCount, pluralize("pod", podCount), total, timeAgo)
}

func (e *Engine) buildSignatureSummary(r string, entries []groupEntry, timeAgo string) string {
	sig := entries[0].logSignature
	byOwner := make(map[string]int)
	for _, ge := range entries {
		o := ge.namespace + "/" + ge.owner
		byOwner[o]++
	}
	owners := make([]string, 0, len(byOwner))
	for o, c := range byOwner {
		if c > 1 {
			owners = append(owners, fmt.Sprintf("%s (%d)", o, c))
		} else {
			owners = append(owners, o)
		}
	}
	sort.Strings(owners)
	total := len(entries)
	return fmt.Sprintf("%s — %s, %s: %s (total %d)%s",
		r, sig, pluralize("deployment", len(byOwner)), strings.Join(owners, ", "), total, timeAgo)
}

func (e *Engine) buildImageSummary(r string, entries []groupEntry, timeAgo string) string {
	img := entries[0].image
	isGlobal := strings.Contains(entries[0].key, "|global|")
	if img == "" {
		img = "unknown"
	}
	if isGlobal {
		byOwner := make(map[string]int)
		for _, ge := range entries {
			o := ge.namespace + "/" + ge.owner
			byOwner[o]++
		}
		ownerCount := len(byOwner)
		total := len(entries)
		return fmt.Sprintf("%s — %d %s affected (total %d)%s",
			r, ownerCount, pluralize("deployment", ownerCount), total, timeAgo)
	}
	byOwner := make(map[string]int)
	for _, ge := range entries {
		byOwner[ge.owner]++
	}
	owners := make([]string, 0, len(byOwner))
	for o, c := range byOwner {
		if c > 1 {
			owners = append(owners, fmt.Sprintf("%s (%d)", o, c))
		} else {
			owners = append(owners, o)
		}
	}
	sort.Strings(owners)
	total := len(entries)
	ns := entries[0].namespace
	if ns != "" {
		ns = " (" + ns + ")"
	}
	return fmt.Sprintf("%s — image %q%s, %s: %s (total %d)%s",
		r, img, ns, pluralize("deployment", len(byOwner)), strings.Join(owners, ", "), total, timeAgo)
}

func (e *Engine) buildNamespaceSummary(r string, entries []groupEntry, timeAgo string) string {
	ns := entries[0].namespace
	byOwner := make(map[string]int)
	for _, ge := range entries {
		byOwner[ge.owner]++
	}
	owners := make([]string, 0, len(byOwner))
	for o, c := range byOwner {
		if c > 1 {
			owners = append(owners, fmt.Sprintf("%s (%d)", o, c))
		} else {
			owners = append(owners, o)
		}
	}
	sort.Strings(owners)
	total := len(entries)
	return fmt.Sprintf("%s — namespace: %s, %s: %s (total %d)%s",
		r, ns, pluralize("deployment", len(byOwner)), strings.Join(owners, ", "), total, timeAgo)
}

func (e *Engine) buildGenericSummary(r string, entries []groupEntry, timeAgo string) string {
	byOwner := make(map[string]int)
	for _, ge := range entries {
		o := ge.namespace + "/" + ge.owner
		byOwner[o]++
	}
	owners := make([]string, 0, len(byOwner))
	for o, c := range byOwner {
		if c > 1 {
			owners = append(owners, fmt.Sprintf("%s (%d)", o, c))
		} else {
			owners = append(owners, o)
		}
	}
	sort.Strings(owners)
	total := len(entries)
	return fmt.Sprintf("%s — affected: %s (total %d)%s",
		r, strings.Join(owners, ", "), total, timeAgo)
}

func (e *Engine) groupSeverity(entries []groupEntry) model.Severity {
	best := model.SeverityNormal
	for _, ge := range entries {
		if inc, ok := e.state[ge.key]; ok {
			if inc.Severity.Rank() > best.Rank() {
				best = inc.Severity
			}
		}
	}
	return best
}

// Caller must hold e.mu.
func (e *Engine) tryConsumeGroupResolve(key string) (groupInc *model.Incident, action model.IncidentAction, tracked bool) {
	for gk, tracker := range e.groupMembers {
		if _, ok := tracker.members[key]; ok {
			tracker.members[key] = true
			allResolved := true
			for _, resolved := range tracker.members {
				if !resolved {
					allResolved = false
					break
				}
			}
			if allResolved {
				delete(e.groupMembers, gk)
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
			return nil, model.ActionSkip, true
		}
	}
	return nil, model.ActionSkip, false
}
