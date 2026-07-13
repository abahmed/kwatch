package correlation

import (
	"context"
	"fmt"
	"hash/crc32"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/abahmed/kwatch/internal/audit"
	"github.com/abahmed/kwatch/internal/enricher"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/metrics"
	"github.com/abahmed/kwatch/internal/model"
	"k8s.io/apimachinery/pkg/labels"
	appsv1lister "k8s.io/client-go/listers/apps/v1"
	corev1lister "k8s.io/client-go/listers/core/v1"
	"k8s.io/klog/v2"
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
	severity    string
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

// BuildKey constructs the incident key used for dedup, grouping, and baseline.
func BuildKey(namespace, owner, reason, container string) string {
	return namespace + ":" + owner + ":" + reason + ":" + container
}

// IncidentKey derives a dedup key from an event, mirroring the exact normalisation
// chain inside Process. It returns the same key that Process would compute.
func IncidentKey(ev event.Event, owner string, cs *model.ContainerState) string {
	r := normalizeReason(ev.Reason)
	// A crash-looping container reports different reasons across its cycle:
	// "Error"/"OOMKilled" when it terminates, "CrashLoopBackOff" while backing off.
	// Once it's established as looping, fold them all into ONE canonical key so the key
	// is stable regardless of the container's momentary state. This makes the startup
	// baseline (captured in whatever state the container was in) match the live alert
	// (fired from a possibly-different state), and treats the loop as a single incident.
	if cs != nil && cs.RestartCount > defaultCrashLoopHighFreqThreshold {
		switch r {
		case "Error", "OOMKilled", "CrashLoopBackOff", "CrashLoopHighFrequency":
			r = "CrashLoopHighFrequency"
		}
	}
	// Cross-namespace dedup: for ImagePullBackOff with global scope (rate limits,
	// timeouts, DNS, TLS errors), use the group key so the same underlying issue
	// maps to a single incident regardless of namespace.
	if r == "ImagePullBackOff" || r == "ErrImagePull" {
		scope := classifyImagePullScope(ev.Message)
		switch scope {
		case "rate_limit", "pull_qps", "timeout", "conn_refused",
			"net_unreachable", "dns", "tls":
			return r + "|global|" + scope
		}
	}
	return BuildKey(ev.Namespace, owner, r, "")
}

func notifSig(inc *model.Incident) string {
	st := "firing"
	if inc.State == model.StateResolved {
		st = "resolved"
	}
	return st + "|" + inc.Severity
}

// edgeAction returns the action to notify, or ActionSkip if nothing changed.
func (e *Engine) edgeAction(inc *model.Incident) model.IncidentAction {
	sig := notifSig(inc)
	if sig == inc.NotifiedSig {
		return model.ActionSkip
	}
	prev := inc.NotifiedSig
	inc.NotifiedSig = sig
	inc.LastNotifiedAt = e.now()
	if inc.State == model.StateResolved {
		metrics.Default.IncidentsResolved.Add(1)
		return model.ActionResolved
	}
	if prev == "" {
		metrics.Default.IncidentsCreate.Add(1)
		return model.ActionCreate
	}
	metrics.Default.IncidentsUpdate.Add(1)
	return model.ActionUpdate
}

// crossedTier returns the highest index of a tier whose threshold was
// crossed when moving from prev to new restarts, or -1.
func crossedTier(prev, new int, tiers []int) int {
	hit := -1
	for i, t := range tiers {
		if prev < t && new >= t {
			hit = i
		}
	}
	return hit
}

// severityForTier returns the severity for the given escalation tier index,
// preferring the higher of the tier-based severity and the current severity.
func severityForTier(tierIdx int, current string) string {
	sev := ""
	switch tierIdx {
	case 0:
		sev = "high"
	default:
		sev = "critical"
	}
	if severityRank(current) > severityRank(sev) {
		return current
	}
	return sev
}

func severityRank(s string) int {
	switch s {
	case "critical":
		return 3
	case "high":
		return 2
	case "medium":
		return 1
	case "normal", "":
		return 0
	default:
		return 0
	}
}

const defaultBaselineTTL = 24 * time.Hour
const defaultCrashLoopHighFreqThreshold = 5
const DefaultMaxBaseline = 2000

type Engine struct {
	mu                  sync.Mutex
	state               map[string]*model.Incident
	namespaceIndex      map[string]map[string]*model.Incident // ns → key → inc
	config              Config
	seen                map[string]map[string]int64
	deployLister        appsv1lister.DeploymentLister
	ssLister            appsv1lister.StatefulSetLister
	dsLister            appsv1lister.DaemonSetLister
	activeNodeIncidents map[string]bool
	lastContainerIndex  map[string]*model.ContainerState // key: namespace/podName
	serviceLister       corev1lister.ServiceLister
	cleanupCooldown     map[string]time.Time     // key → cooldown expiry; prevents resolve→recreate cycle
	pendingGroups       map[string]*pendingGroup         // computeGroupKey output → group buffer
	groupMembers        map[string]*groupResolveTracker   // gk → batch resolve tracker
	auditLogger         *audit.AuditLogger
	dirty               bool // true when state has changed since last SnapshotAll
	now                 func() time.Time
}

func NewEngine(cfg Config) *Engine {
	if cfg.Enricher == nil {
		cfg.Enricher = &enricher.DefaultEnricher{}
	}
	if cfg.LifecycleInterval <= 0 {
		cfg.LifecycleInterval = 1 * time.Minute
	}
	if cfg.BaselineTTL <= 0 {
		cfg.BaselineTTL = defaultBaselineTTL
	}
	if cfg.MaxBaseline <= 0 {
		cfg.MaxBaseline = DefaultMaxBaseline
	}
	e := &Engine{
		state:               make(map[string]*model.Incident),
		namespaceIndex:      make(map[string]map[string]*model.Incident),
		config:              cfg,
		activeNodeIncidents: make(map[string]bool),
		lastContainerIndex:  make(map[string]*model.ContainerState),
		cleanupCooldown:     make(map[string]time.Time),
		pendingGroups:       make(map[string]*pendingGroup),
		groupMembers:        make(map[string]*groupResolveTracker),
	}
	if e.now == nil {
		e.now = time.Now
	}
	if cfg.Baseline != nil {
		e.SetSeen(cfg.Baseline)
	}
	return e
}

func (e *Engine) SetSeen(b map[string]map[string]int64) {
	e.mu.Lock()
	e.dirty = true
	now := e.now()
	ttl := e.config.BaselineTTL
	if e.seen == nil {
		e.seen = make(map[string]map[string]int64)
	}
	for key, pods := range b {
		for pod, ts := range pods {
			if now.Sub(time.Unix(ts, 0)) < ttl {
				if e.seen[key] == nil {
					e.seen[key] = map[string]int64{}
				}
				e.seen[key][pod] = ts
			}
		}
	}
	e.evictToLimit()
	snap := cloneBaseline(e.seen)
	e.mu.Unlock()
	if e.config.OnBaselineChange != nil {
		e.config.OnBaselineChange(snap)
	}
}

type seenEntry struct {
	key string
	pod string
	ts  int64
}

// Caller must hold e.mu.
func (e *Engine) evictToLimit() {
	limit := e.config.MaxBaseline
	total := 0
	for _, pods := range e.seen {
		total += len(pods)
	}
	if total <= limit {
		return
	}

	var all []seenEntry
	for key, pods := range e.seen {
		for pod, ts := range pods {
			all = append(all, seenEntry{key, pod, ts})
		}
	}
	sort.Slice(all, func(i, j int) bool {
		return all[i].ts < all[j].ts
	})

	toRemove := total - limit
	for _, entry := range all[:toRemove] {
		if pods, ok := e.seen[entry.key]; ok {
			delete(pods, entry.pod)
			if len(pods) == 0 {
				delete(e.seen, entry.key)
			}
		}
	}
}

// Caller must hold e.mu.
func (e *Engine) isBaselined(key, podName string) bool {
	if pods, ok := e.seen[key]; ok {
		if ts, ok := pods[podName]; ok {
			if e.now().Sub(time.Unix(ts, 0)) < e.config.BaselineTTL {
				return true
			}
			delete(pods, podName)
			if len(pods) == 0 {
				delete(e.seen, key)
			}
		}
	}
	return false
}

// ClearSeenForPod removes all baseline entries and cooldowns for the given pod.
func (e *Engine) ClearSeenForPod(namespace, podName string) {
	e.mu.Lock()
	e.dirty = true
	changed := false
	for key, pods := range e.seen {
		if !strings.HasPrefix(key, namespace+":") {
			continue
		}
		if _, ok := pods[podName]; ok {
			delete(pods, podName)
			changed = true
			if len(pods) == 0 {
				delete(e.seen, key)
			}
		}
	}
	// Clear any cleanup cooldown entries for this namespace so a recovered
	// pod that re-crashes isn't suppressed by a stale cooldown.
	nsPrefix := namespace + ":"
	for key := range e.cleanupCooldown {
		if strings.HasPrefix(key, nsPrefix) {
			delete(e.cleanupCooldown, key)
		}
	}
	var snap map[string]map[string]int64
	if changed {
		snap = cloneBaseline(e.seen)
	}
	e.mu.Unlock()
	if changed && e.config.OnBaselineChange != nil {
		e.config.OnBaselineChange(snap)
	}
}

func cloneBaseline(src map[string]map[string]int64) map[string]map[string]int64 {
	dst := make(map[string]map[string]int64, len(src))
	for k, pods := range src {
		m := make(map[string]int64, len(pods))
		for p, ts := range pods {
			m[p] = ts
		}
		dst[k] = m
	}
	return dst
}

func (e *Engine) ActiveCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	n := 0
	for _, inc := range e.state {
		if inc.State != model.StateResolved {
			n++
		}
	}
	return n
}

func (e *Engine) Snapshot() []model.IncidentView {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]model.IncidentView, 0, len(e.state))
	for _, inc := range e.state {
		out = append(out, model.IncidentView{
			Key:       inc.Key,
			Reason:    inc.Reason,
			Namespace: inc.Namespace,
			Name:      inc.Name,
			State:     inc.State,
			Severity:  inc.Severity,
			Count:     inc.Count,
			FirstSeen: inc.FirstSeen,
			LastSeen:  inc.LastSeen,
			Hint:      inc.Hint,
		})
	}
	return out
}

var knownRetryReasons = map[string]bool{
	"CrashLoopBackOff": true,
	"BackOff":          true,
	"ErrImagePull":     true,
	"ImagePullBackOff": true,
}

func normalizeReason(reason string) string {
	if reason == "ErrImagePull" {
		return "ImagePullBackOff"
	}
	idx := strings.LastIndex(reason, " ")
	if idx > 0 {
		base, suffix := reason[:idx], reason[idx+1:]
		if _, err := strconv.Atoi(suffix); err == nil && knownRetryReasons[base] {
			if base == "ErrImagePull" {
				return "ImagePullBackOff"
			}
			return base
		}
	}
	return reason
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
	case "OOMKilled", "OOMRepeating", "CrashLoopHighFrequency", "HighRestartCount",
		"InitContainerError",
		"ContainerCannotRun", "CreateContainerError",
		"DeadlineExceeded",
		"StartupProbeFailed", "LivenessProbeFailed",
		"ReadinessProbeFailed", "ProbeError",
		"PostStartHookError", "PreStopHookError",
		"NodeAffinity",
		"ProgressDeadlineExceeded", "DeploymentUnavailable",
		"DaemonSetUnavailable",
		"StsUnavailable",
		"PdbViolation",
		"HPAMaxedOut", "HPAScalingError",
		"JobFailed", "JobSuspended",
		"CronJobSuspended", "CronJobNotScheduled",
		"VolumeUsageHigh",
		"PreExistingAtStartup":
		return r + "|" + ev.Namespace + "|" + owner

	case "CrashLoopBackOff", "BackOff", "Error":
		if sig := enricher.SignatureHint(ev.Logs); sig != "" {
			return r + "|sig|" + sig
		}
		return r + "|" + ev.Namespace + "|" + owner

	case "ImagePullBackOff", "ErrImagePull":
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

	case "ImageInspectError", "InvalidImageName":
		return r + "|img|" + ev.Image + "|ns|" + ev.Namespace

	case "NodeNotReady", "MemoryPressure", "DiskPressure",
		"PIDPressure", "NetworkUnavailable",
		"ContainerStatusUnknown", "Evicted", "Preempting",
		"NodeResourceHigh", "NodeResourceCritical":
		return r + "|node|" + ev.NodeName

	case "ServiceNoEndpoints":
		return r + "|svc|" + ev.Namespace + "/" + ev.PodName

	case "ControlPlaneComponentFailure":
		return r + "|cp|" + ev.Namespace

	case "CreateContainerConfigError", "Unschedulable", "PodPending",
		"SchedulingGated",
		"RegistryUnavailable",
		"TLSCertExpired", "TLSCertExpiringSoon":
		return r + "|ns|" + ev.Namespace

	default:
		return r + "|" + ev.Namespace + "|" + owner
	}
}

// Caller must hold e.mu.
func (e *Engine) findNodeIncident(nodeName string) *model.Incident {
	for _, inc := range e.state {
		if inc.Resource == "node" && inc.Name == nodeName {
			return inc
		}
	}
	return nil
}

// findMostConstrainedNodeIncident returns the node incident with the most
// suppressed pods, used as a target for unschedulable-pod suppression.
// Caller must hold e.mu.
func (e *Engine) findMostConstrainedNodeIncident() *model.Incident {
	var best *model.Incident
	for _, inc := range e.state {
		if inc.Resource == "node" && inc.State != model.StateResolved {
			if best == nil || inc.SuppressedPods > best.SuppressedPods {
				best = inc
			}
		}
	}
	return best
}

// CountActiveNodeIncidents returns the number of nodes with active
// (non-resolved) incidents. Used for node→resource inhibition decisions.
func (e *Engine) CountActiveNodeIncidents() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.activeNodeIncidents)
}

// SetActiveNodeIncidents marks the given nodes as having active incidents.
// Used at startup to pre-populate inhibition before any worker runs.
func (e *Engine) SetActiveNodeIncidents(nodeNames []string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, n := range nodeNames {
		e.activeNodeIncidents[n] = true
	}
}

// refreshNodeInhibition clears the node inhibition flag if no non-resolved
// node incidents remain for this node. Caller must hold e.mu.
func (e *Engine) refreshNodeInhibition(nodeName string) {
	for _, inc := range e.state {
		if inc.Resource == "node" && inc.Name == nodeName && inc.State != model.StateResolved {
			return
		}
	}
	delete(e.activeNodeIncidents, nodeName)
}

func (e *Engine) GetLastContainerState(namespace, podName, _ string) *model.ContainerState {
	e.mu.Lock()
	defer e.mu.Unlock()
	cs, ok := e.lastContainerIndex[namespace+"/"+podName]
	if !ok || cs == nil {
		return nil
	}
	cp := *cs
	return &cp
}

// Caller must hold e.mu.
func (e *Engine) indexLastContainerState(namespace, podName string, cs *model.ContainerState) {
	if podName == "" || cs == nil {
		return
	}
	cp := *cs
	e.lastContainerIndex[namespace+"/"+podName] = &cp
}

// Caller must hold e.mu.
func (e *Engine) indexIncidentByNamespace(inc *model.Incident) {
	ns, key := inc.Namespace, inc.Key
	if ns == "" {
		return
	}
	if e.namespaceIndex[ns] == nil {
		e.namespaceIndex[ns] = make(map[string]*model.Incident)
	}
	e.namespaceIndex[ns][key] = inc
}

// Caller must hold e.mu.
func (e *Engine) removeIncidentFromNamespaceIndex(inc *model.Incident) {
	ns, key := inc.Namespace, inc.Key
	if ns == "" {
		return
	}
	delete(e.namespaceIndex[ns], key)
	if len(e.namespaceIndex[ns]) == 0 {
		delete(e.namespaceIndex, ns)
	}
}

func (e *Engine) SetAnalysis(key, analysis string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if inc, ok := e.state[key]; ok {
		inc.Analysis = analysis
	}
}

func (e *Engine) SetDeployLister(l appsv1lister.DeploymentLister) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.deployLister = l
}

func (e *Engine) SetStatefulSetLister(l appsv1lister.StatefulSetLister) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.ssLister = l
}

func (e *Engine) SetDaemonSetLister(l appsv1lister.DaemonSetLister) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.dsLister = l
}

func (e *Engine) SetServiceLister(l corev1lister.ServiceLister) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.serviceLister = l
}

func (e *Engine) SetAuditLogger(l *audit.AuditLogger) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.auditLogger = l
}

// SnapshotAll returns a deep copy of all non-resolved incidents keyed by ID.
func (e *Engine) SnapshotAll() map[string]*model.Incident {
	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.dirty {
		return nil
	}
	out := make(map[string]*model.Incident, len(e.state))
	for key, inc := range e.state {
		if inc.State == model.StateResolved {
			continue
		}
		out[key] = inc.Clone()
	}
	e.dirty = false
	return out
}

// RestoreIncidents loads previously persisted incidents into the state map.
// Only incidents whose key still exists in the seen (baseline) set are
// restored, to avoid re-alerting for issues that were resolved while down.
// LastSeen is bumped to now to prevent immediate cleanup-loop resolution.
func (e *Engine) RestoreIncidents(incidents map[string]*model.Incident) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.dirty = true
	if len(incidents) == 0 {
		return
	}
	now := e.now()
	restored := 0
	for key, inc := range incidents {
		if _, ok := e.seen[key]; !ok || len(e.seen[key]) == 0 {
			continue
		}
		if _, exists := e.state[key]; exists {
			continue
		}
		inc.LastSeen = now
		inc.LastUpdate = now
		inc.NotifiedSig = notifSig(inc)
		e.state[key] = inc
		e.indexIncidentByNamespace(inc)
		restored++
	}
	if restored > 0 {
		klog.InfoS("restored incidents from ConfigMap", "count", restored)
	}
}

// findDependentServices returns the names of Services in the given namespace
// whose selectors match the provided pod labels. Returns nil if no service
// lister is configured or no matches are found.
func (e *Engine) findDependentServices(namespace string, podLabels map[string]string) []string {
	if e.serviceLister == nil || len(podLabels) == 0 {
		return nil
	}
	svcs, err := e.serviceLister.Services(namespace).List(labels.Everything())
	if err != nil {
		return nil
	}
	var result []string
	for _, svc := range svcs {
		if len(svc.Spec.Selector) == 0 {
			continue
		}
		match := true
		for k, v := range svc.Spec.Selector {
			if podLabels[k] != v {
				match = false
				break
			}
		}
		if match {
			result = append(result, svc.Name)
		}
	}
	return result
}

func (e *Engine) isOwnerHealthy(inc *model.Incident) bool {
	if inc.Resource != "pod" {
		return true
	}
	ns := inc.Namespace
	name := inc.Name
	if ns == "" || name == "" {
		return true
	}

	switch inc.OwnerKind {
	case "Deployment":
		if e.deployLister == nil {
			return true
		}
		d, err := e.deployLister.Deployments(ns).Get(name)
		if err != nil {
			return len(inc.Resources) == 0
		}
		if d.Status.ObservedGeneration < d.Generation {
			return false
		}
		return d.Status.ReadyReplicas >= d.Status.Replicas &&
			d.Status.UnavailableReplicas == 0

	case "StatefulSet":
		if e.ssLister == nil {
			return true
		}
		ss, err := e.ssLister.StatefulSets(ns).Get(name)
		if err != nil {
			return len(inc.Resources) == 0
		}
		if ss.Status.ObservedGeneration < ss.Generation {
			return false
		}
		return ss.Status.ReadyReplicas >= ss.Status.Replicas &&
			ss.Status.CurrentRevision == ss.Status.UpdateRevision

	case "DaemonSet":
		if e.dsLister == nil {
			return true
		}
		ds, err := e.dsLister.DaemonSets(ns).Get(name)
		if err != nil {
			return len(inc.Resources) == 0
		}
		return ds.Status.DesiredNumberScheduled > 0 &&
			ds.Status.NumberUnavailable == 0 &&
			ds.Status.UpdatedNumberScheduled == ds.Status.DesiredNumberScheduled

	default:
		return true
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
	if r == "CrashLoopBackOff" || r == "BackOff" || r == "Error" {
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

func (e *Engine) Process(ev event.Event, owner string, cs *model.ContainerState) (incident *model.Incident, action model.IncidentAction) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.dirty = true
	defer func() {
		if incident != nil {
			incident = incident.Clone()
		}
	}()

	key := IncidentKey(ev, owner, cs)

	res := ev.Resource
	if res == "" {
		res = "pod"
	}

	// Track active node incidents for pod suppression — must happen before
	// baseline check so node events always populate the inhibition map.
	if res == "node" && ev.NodeName != "" {
		e.activeNodeIncidents[ev.NodeName] = true
	}

	// Baseline — skip for node events so the incident is always created
	if res != "node" && e.isBaselined(key, ev.PodName) {
		if e.auditLogger != nil {
			e.auditLogger.LogSkip(&model.Incident{Key: key, Namespace: ev.Namespace, Reason: ev.Reason, ID: key}, "baseline")
		}
		return nil, model.ActionSkip
	}

	// Suppress pod incidents when the node has an active incident
	if e.config.InhibitNodeSuppressesPods && res == "pod" {
		if ev.NodeName != "" && e.activeNodeIncidents[ev.NodeName] {
			if nodeInc := e.findNodeIncident(ev.NodeName); nodeInc != nil {
				nodeInc.SuppressedPods++
				if owner != "" {
					if nodeInc.SuppressedOwners == nil {
						nodeInc.SuppressedOwners = make(map[string]int)
					}
					nodeInc.SuppressedOwners[owner]++
				}
				if len(nodeInc.SuppressedPodSummaries) < 20 {
					nodeInc.SuppressedPodSummaries = append(nodeInc.SuppressedPodSummaries, model.PodSummary{
						Namespace:    ev.Namespace,
						PodName:      ev.PodName,
						Reason:       ev.Reason,
						RestartCount: ev.RestartCount,
					})
				}
			}
			if e.auditLogger != nil {
				e.auditLogger.LogSkip(&model.Incident{Key: key, Namespace: ev.Namespace, Reason: ev.Reason, ID: key, NodeName: ev.NodeName}, "node_inhibition")
			}
			return nil, model.ActionSkip
		}
		// Unschedulable pods (empty NodeName) — suppress when any node incident is active
		if ev.NodeName == "" && len(e.activeNodeIncidents) > 0 {
			if nodeInc := e.findMostConstrainedNodeIncident(); nodeInc != nil {
				nodeInc.SuppressedPods++
				if owner != "" {
					if nodeInc.SuppressedOwners == nil {
						nodeInc.SuppressedOwners = make(map[string]int)
					}
					nodeInc.SuppressedOwners[owner]++
				}
				if len(nodeInc.SuppressedPodSummaries) < 20 {
					nodeInc.SuppressedPodSummaries = append(nodeInc.SuppressedPodSummaries, model.PodSummary{
						Namespace:    ev.Namespace,
						PodName:      ev.PodName,
						Reason:       ev.Reason,
						RestartCount: ev.RestartCount,
					})
				}
			}
			if e.auditLogger != nil {
				e.auditLogger.LogSkip(&model.Incident{Key: key, Namespace: ev.Namespace, Reason: ev.Reason, ID: key}, "node_inhibition")
			}
			return nil, model.ActionSkip
		}
	}

	// Cooldown check — suppress re-creation after cleanup for still-broken resources
	if expiry, ok := e.cleanupCooldown[key]; ok {
		if e.now().Before(expiry) {
			if e.auditLogger != nil {
				e.auditLogger.LogSkip(&model.Incident{Key: key, Namespace: ev.Namespace, Reason: ev.Reason, ID: key}, "cooldown")
			}
			return nil, model.ActionSkip
		}
		delete(e.cleanupCooldown, key)
	}

	now := e.now()

	// Cascading suppression: if a pod incident fires and its owning workload
	// already has an active (non-pod) incident, suppress the pod as a symptom.
	if res == "pod" && owner != "" {
		prefix := ev.Namespace + ":" + owner + ":"
		for _, existing := range e.state {
			if existing.State == model.StateResolved ||
				existing.State == model.StatePendingResolve {
				continue
			}
			if existing.Resource != "pod" &&
				existing.Namespace == ev.Namespace &&
				existing.Name == owner &&
				strings.HasPrefix(existing.Key, prefix) {
				existing.Count++
				if ev.PodName != "" {
					existing.Resources[ev.PodName] = true
					if len(existing.Resources) > existing.PeakResources {
						existing.PeakResources = len(existing.Resources)
					}
				}
				existing.LastSeen = now
				if e.auditLogger != nil {
					e.auditLogger.LogSkip(&model.Incident{Key: key, Namespace: ev.Namespace, Reason: ev.Reason, ID: key}, "cascading_suppression")
				}
				return nil, model.ActionSkip
			}
		}
	}

	if inc, ok := e.state[key]; ok {
		// Already resolved — silently revive instead of re-creating.
		// Re-creating would emit a CREATE notification, causing a
		// resolved→CREATE→resolved flip-flop cycle. Silent revival
		// keeps the existing incident active and returns ActionUpdate.
		if inc.State == model.StateResolved {
			inc.State = model.StateActive
			inc.ResolveAt = time.Time{}
			if ev.PodName != "" {
				inc.Resources[ev.PodName] = true
				if len(inc.Resources) > inc.PeakResources {
					inc.PeakResources = len(inc.Resources)
				}
			}
			if ev.ContainerName != "" && ev.ContainerName != "." {
				inc.Containers[ev.ContainerName] = true
			}
			inc.LastContainerState = cs
			e.indexLastContainerState(ev.Namespace, ev.PodName, cs)
			if cs != nil {
				inc.RestartCount = int(cs.RestartCount)
			}
			inc.Count++
			inc.LastSeen = now
			inc.LastUpdate = now
			e.config.Enricher.Enrich(&ev, inc)
			if e.tryGroupIncident(inc, ev, owner, now) {
				return inc, model.ActionSkip
			}
			return inc, e.edgeAction(inc)
		}

		// Pending resolve — revoke the scheduled resolve
		if inc.State == model.StatePendingResolve {
			inc.State = model.StateActive
			inc.ResolveAt = time.Time{}
			if ev.PodName != "" {
				inc.Resources[ev.PodName] = true
				if len(inc.Resources) > inc.PeakResources {
					inc.PeakResources = len(inc.Resources)
				}
			}
			if ev.ContainerName != "" && ev.ContainerName != "." {
				inc.Containers[ev.ContainerName] = true
			}
			inc.LastContainerState = cs
			e.indexLastContainerState(ev.Namespace, ev.PodName, cs)
			if cs != nil {
				inc.RestartCount = int(cs.RestartCount)
			}
			inc.Count++
			inc.LastSeen = now
			inc.LastUpdate = now
			e.config.Enricher.Enrich(&ev, inc)
			if e.tryGroupIncident(inc, ev, owner, now) {
				return inc, model.ActionSkip
			}
			return inc, e.edgeAction(inc)
		}

		if e.config.EscalationEnabled && cs != nil {
			prev := inc.RestartCount
			cur := int(cs.RestartCount)
			if t := crossedTier(prev, cur, e.config.EscalationTiers); t >= 0 {
				ev.Severity = severityForTier(t, inc.Severity)
				e.config.Enricher.Enrich(&ev, inc)
				inc.Hint = fmt.Sprintf("restart count crossed %d", e.config.EscalationTiers[t])
				inc.Count++
				inc.LastSeen = now
				inc.State = model.StateActive
				inc.LastUpdate = now
				inc.RestartCount = cur
				if ev.PodName != "" {
					inc.Resources[ev.PodName] = true
					if len(inc.Resources) > inc.PeakResources {
						inc.PeakResources = len(inc.Resources)
					}
				}
				if ev.ContainerName != "" && ev.ContainerName != "." {
					inc.Containers[ev.ContainerName] = true
				}
				inc.LastContainerState = cs
				e.indexLastContainerState(ev.Namespace, ev.PodName, cs)
				return inc, e.edgeAction(inc)
			}
		}
		inc.Count++
		inc.LastSeen = now
		inc.State = model.StateActive
		inc.LastUpdate = now
		if ev.PodName != "" {
			inc.Resources[ev.PodName] = true
			if len(inc.Resources) > inc.PeakResources {
				inc.PeakResources = len(inc.Resources)
			}
		}
		if ev.ContainerName != "" && ev.ContainerName != "." {
			inc.Containers[ev.ContainerName] = true
		}
		inc.LastContainerState = cs
		e.indexLastContainerState(ev.Namespace, ev.PodName, cs)
		if cs != nil {
			inc.RestartCount = int(cs.RestartCount)
		}
		e.config.Enricher.Enrich(&ev, inc)
		if e.tryGroupIncident(inc, ev, owner, now) {
			return inc, model.ActionSkip
		}
		return inc, e.edgeAction(inc)
	}

	// When RestartCount crosses the CrashLoopHighFrequency threshold, the
	// incident key changes from the raw reason (e.g. "Error") to
	// "CrashLoopHighFrequency", orphaning the old incident in the state map.
	// Silently resolve any orphaned incidents with the same ns:owner: prefix
	// so the state map doesn't accumulate stale entries for the same pod.
	if cs != nil && int(cs.RestartCount) > defaultCrashLoopHighFreqThreshold {
		prefix := ev.Namespace + ":" + owner + ":"
		for k, oldInc := range e.state {
			if strings.HasPrefix(k, prefix) && k != key &&
				oldInc.State != model.StateResolved &&
				oldInc.State != model.StatePendingResolve &&
				oldInc.Resource == res {
				oldInc.State = model.StateResolved
				e.removeIncidentFromNamespaceIndex(oldInc)
				delete(e.state, k)
				delete(e.seen, k)
			}
		}
	}

	inc := e.newIncident(ev, owner, cs, key, res, now)
	e.state[key] = inc
	e.indexIncidentByNamespace(inc)

	// Smart grouping: buffer same-reason incidents, suppress individual
	// notification until the group window expires.
	if e.tryGroupIncident(inc, ev, owner, now) {
		return inc, model.ActionSkip
	}

	return inc, e.edgeAction(inc)
}

// Caller must hold e.mu.
func (e *Engine) newIncident(ev event.Event, owner string, cs *model.ContainerState, key, res string, now time.Time) *model.Incident {
	inc := &model.Incident{
		ID:         fmt.Sprintf("%08x", crc32.ChecksumIEEE([]byte(key))),
		Key:        key,
		Reason:     ev.Reason,
		Namespace:  ev.Namespace,
		Resource:   res,
		Name:       owner,
		NodeName:   ev.NodeName,
		Count:      1,
		FirstSeen:  now,
		LastSeen:   now,
		LastUpdate: now,
		State:      model.StateActive,
		Resources:  map[string]bool{},
		Containers: map[string]bool{},
	}
	if ev.PodName != "" {
		inc.Resources[ev.PodName] = true
	}
	inc.PeakResources = len(inc.Resources)
	if ev.ContainerName != "" && ev.ContainerName != "." {
		inc.Containers[ev.ContainerName] = true
	}
	inc.LastContainerState = cs
	e.indexLastContainerState(ev.Namespace, ev.PodName, cs)
	if cs != nil {
		inc.RestartCount = int(cs.RestartCount)
	}
	if url, ok := e.config.Runbooks[ev.Reason]; ok {
		inc.Runbook = url
	}
	if e.config.EscalationEnabled && cs != nil {
		cur := int(cs.RestartCount)
		if t := crossedTier(-1, cur, e.config.EscalationTiers); t >= 0 {
			ev.Severity = severityForTier(t, inc.Severity)
		} else if ev.Severity == "" {
			// seed from the absolute threshold when no tier is crossed at startup
			for i := len(e.config.EscalationTiers) - 1; i >= 0; i-- {
				if cur >= e.config.EscalationTiers[i] {
					ev.Severity = severityForTier(i, inc.Severity)
					break
				}
			}
		}
	}
	e.config.Enricher.Enrich(&ev, inc)

	// Topological annotation: dependent services
	if deps := e.findDependentServices(ev.Namespace, ev.Labels); len(deps) > 0 {
		inc.Hint = enricher.CombineHints(inc.Hint,
			fmt.Sprintf("affects service(s): %s", strings.Join(deps, ", ")))
	}

	// Topological annotation: parent workload health
	if res == "pod" && owner != "" && !e.isOwnerHealthy(inc) {
		inc.Hint = enricher.CombineHints(inc.Hint,
			fmt.Sprintf("owning %s %s is also unhealthy", ev.OwnerKind, owner))
	}

	return inc
}

func (e *Engine) MarkResolved(key string) {
	e.mu.Lock()
	e.dirty = true
	inc, ok := e.state[key]
	if !ok || inc.State == model.StateResolved || inc.State == model.StatePendingResolve {
		e.mu.Unlock()
		return
	}
	// Do not resolve if the owning workload is still unhealthy.
	if !e.isOwnerHealthy(inc) {
		e.mu.Unlock()
		return
	}
	if e.config.ResolveHoldDown > 0 {
		inc.State = model.StatePendingResolve
		inc.ResolveAt = e.now().Add(e.config.ResolveHoldDown)
		e.mu.Unlock()
		return
	}
	// Smart group batch resolve: check if this incident is a member of a
	// tracked smart group. If so, buffer the resolve and only emit one
	// notification when all members have resolved.
	if groupInc, groupAction, tracked := e.tryConsumeGroupResolve(key); tracked {
		inc.State = model.StateResolved
		if inc.Resource == "node" {
			e.refreshNodeInhibition(inc.Name)
		}
		delete(e.seen, key)
		e.mu.Unlock()
		if groupAction != model.ActionSkip {
			if hook := e.config.LifecycleHook; hook != nil {
				hook(groupInc.Clone(), groupAction)
			}
		}
		if hook := e.config.OnBaselineChange; hook != nil {
			e.mu.Lock()
			snapshot := cloneBaseline(e.seen)
			e.mu.Unlock()
			hook(snapshot)
		}
		return
	}
	inc.State = model.StateResolved
	if inc.Resource == "node" {
		e.refreshNodeInhibition(inc.Name)
	}
	delete(e.seen, key)
	action := e.edgeAction(inc)
	snap := inc.Clone()
	e.mu.Unlock()

	if action != model.ActionSkip {
		if hook := e.config.LifecycleHook; hook != nil {
			hook(snap, action)
		}
	}
	if hook := e.config.OnBaselineChange; hook != nil {
		e.mu.Lock()
		snapshot := cloneBaseline(e.seen)
		e.mu.Unlock()
		hook(snapshot)
	}
}

func (e *Engine) RemovePod(namespace, podName string) {
	var baselineChanged bool

	e.mu.Lock()
	e.dirty = true
	for _, inc := range e.state {
		if inc.Namespace != namespace {
			continue
		}
		if !inc.Resources[podName] {
			continue
		}
		delete(inc.Resources, podName)
		// Pod removal does NOT resolve incidents. During a crash loop, the
		// ReplicaSet replaces pods continuously and each deletion would
		// resolve the incident, then the new pod would re-create it, causing
		// a flip-flop cycle. Resolution is handled solely by cleanup(),
		// checkLifecycle(), and MarkResolved().
	}
	// Release per-pod baseline slots for this pod
	for key, pods := range e.seen {
		if _, ok := pods[podName]; ok {
			delete(pods, podName)
			baselineChanged = true
			if len(pods) == 0 {
				delete(e.seen, key)
			}
		}
	}
	delete(e.lastContainerIndex, namespace+"/"+podName)
	e.mu.Unlock()

	if baselineChanged {
		if hook := e.config.OnBaselineChange; hook != nil {
			e.mu.Lock()
			snapshot := cloneBaseline(e.seen)
			e.mu.Unlock()
			hook(snapshot)
		}
	}
}

func (e *Engine) ResolveByResource(resource, name string) {
	type transition struct {
		inc    *model.Incident
		action model.IncidentAction
	}
	var pending []transition
	var baselineChanged bool

	e.mu.Lock()
	e.dirty = true
	now := e.now()
	for key, inc := range e.state {
		if inc.Resource == resource && inc.Name == name && inc.State != model.StateResolved {
			if inc.State == model.StatePendingResolve {
				continue
			}
			// For pod incidents owned by a workload, gate on workload health.
			if !e.isOwnerHealthy(inc) {
				continue
			}
			if e.config.ResolveHoldDown > 0 {
				inc.State = model.StatePendingResolve
				inc.ResolveAt = now.Add(e.config.ResolveHoldDown)
				e.cleanupCooldown[key] = now.Add(e.config.Window)
				continue
			}
			inc.State = model.StateResolved
			if inc.Resource == "node" {
				e.refreshNodeInhibition(inc.Name)
			}
			delete(e.seen, key)
			e.cleanupCooldown[key] = now.Add(e.config.Window)
			action := e.edgeAction(inc)
			baselineChanged = true
			pending = append(pending, transition{inc.Clone(), action})
		}
	}
	e.mu.Unlock()

	for _, t := range pending {
		if hook := e.config.LifecycleHook; hook != nil {
			hook(t.inc, t.action)
		}
	}
	if baselineChanged {
		if hook := e.config.OnBaselineChange; hook != nil {
			e.mu.Lock()
			snapshot := cloneBaseline(e.seen)
			e.mu.Unlock()
			hook(snapshot)
		}
	}
}

func (e *Engine) StartCleanup(ctx context.Context) {
	cleanupInterval := e.config.Window / 2
	if cleanupInterval < 30*time.Second {
		cleanupInterval = 30 * time.Second
	}
	cleanupTicker := time.NewTicker(cleanupInterval)
	defer cleanupTicker.Stop()

	lifecycleTicker := time.NewTicker(e.config.LifecycleInterval)
	defer lifecycleTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			klog.InfoS("correlation cleanup stopped")
			return
		case <-cleanupTicker.C:
			e.cleanup()
		case <-lifecycleTicker.C:
			e.checkLifecycle()
		}
	}
}

func (e *Engine) cleanup() {
	e.mu.Lock()
	e.dirty = true
	now := e.now()
	type transition struct {
		inc    *model.Incident
		action model.IncidentAction
	}
	var pending []transition
	for key, inc := range e.state {
		if !now.After(inc.LastSeen.Add(e.config.Window)) {
			continue
		}
		// Do not clean up pod incidents whose owning workload is still unhealthy.
		if !e.isOwnerHealthy(inc) {
			continue
		}
		// Finalize active/digested incidents with a resolve so the
		// LifecycleHook emits a resolved notification and Slack's
		// threadMap is pruned. Skip StatePendingResolve — that state is
		// owned by checkLifecycle.
		if inc.State != model.StateResolved && inc.State != model.StatePendingResolve {
			inc.State = model.StateResolved
			// Smart group batch resolve
			if groupInc, groupAction, tracked := e.tryConsumeGroupResolve(key); tracked {
				if groupAction != model.ActionSkip {
					pending = append(pending, transition{groupInc, groupAction})
				}
			} else if a := e.edgeAction(inc); a != model.ActionSkip {
				pending = append(pending, transition{inc.Clone(), a})
			}
		}
		// Add cooldown to prevent resolve→recreate cycle for still-broken resources
		e.cleanupCooldown[key] = now.Add(e.config.Window)
		delete(e.seen, key)
		e.removeIncidentFromNamespaceIndex(inc)
		delete(e.state, key)
		if inc.Resource == "node" {
			e.refreshNodeInhibition(inc.Name)
		}
	}
	e.mu.Unlock()
	for _, t := range pending {
		if h := e.config.LifecycleHook; h != nil {
			h(t.inc, t.action)
		}
	}
}

func (e *Engine) checkLifecycle() {
	type transition struct {
		inc    *model.Incident
		action model.IncidentAction
	}
	var pending []transition
	var baselineChanged bool

	e.mu.Lock()
	e.dirty = true
	now := e.now()

	// pending resolve finalization
	for key, inc := range e.state {
		if inc.State == model.StatePendingResolve && !inc.ResolveAt.IsZero() && now.After(inc.ResolveAt) {
			// Do not finalize if the owning workload is still unhealthy.
			if !e.isOwnerHealthy(inc) {
				inc.State = model.StateActive
				inc.ResolveAt = time.Time{}
				continue
			}
			inc.State = model.StateResolved
			if inc.Resource == "node" {
				e.refreshNodeInhibition(inc.Name)
			}
			delete(e.seen, key)
			e.cleanupCooldown[key] = now.Add(e.config.Window)
			// Smart group batch resolve
			if groupInc, groupAction, tracked := e.tryConsumeGroupResolve(key); tracked {
				baselineChanged = true
				if groupAction != model.ActionSkip {
					pending = append(pending, transition{groupInc, groupAction})
				}
			} else {
				action := e.edgeAction(inc)
				baselineChanged = true
				pending = append(pending, transition{inc.Clone(), action})
			}
		}
	}

	// renotify — resend on time-based interval (not stale-gated)
	renotifyBySev := e.config.RenotifyIntervalBySeverity
	if len(renotifyBySev) > 0 {
		for _, inc := range e.state {
			if inc.State == model.StateResolved || inc.State == model.StatePendingResolve {
				continue
			}
			maxPer := e.config.RenotifyMaxPerIncident
			if maxPer <= 0 {
				maxPer = 3
			}
			if inc.RenotifyCount >= maxPer {
				continue
			}
			interval, ok := renotifyBySev[inc.Severity]
			if !ok || interval <= 0 {
				interval, ok = renotifyBySev["default"]
			}
			if !ok || interval <= 0 {
				continue
			}
			if now.After(inc.LastNotifiedAt.Add(interval)) {
				inc.RenotifyCount++
				inc.LastNotifiedAt = now
				// For renotify we emit update
				pending = append(pending, transition{inc.Clone(), model.ActionUpdate})
			}
		}
	}

	// smart grouping flush
	if e.config.SmartGroupingWindow > 0 {
		for gk, pg := range e.pendingGroups {
			if len(pg.entries) > 0 && now.After(pg.firstSeen.Add(e.config.SmartGroupingWindow)) {
				var active []groupEntry
				for _, ge := range pg.entries {
					if inc, ok := e.state[ge.key]; ok && inc.State != model.StateResolved && inc.State != model.StatePendingResolve {
						active = append(active, ge)
					}
				}
				if len(active) > 0 {
					summary := e.buildGroupSummary(active, pg.firstSeen)
					if pg.overflowCount > 0 {
						summary += fmt.Sprintf(" +%d more", pg.overflowCount)
					}
					groupIncKey := "__group__:" + gk + ":" + strconv.FormatInt(now.Unix(), 10)
					sev := e.groupSeverity(active)
				// Copy rich data (logs, events, analysis, runbook) from the first
				// member incident so the group notification includes diagnostics.
				resources := make(map[string]bool)
				for _, ge := range active {
					if ge.podName != "" {
						resources[ge.podName] = true
					}
				}
				groupInc := &model.Incident{
					ID:        fmt.Sprintf("%08x", crc32.ChecksumIEEE([]byte(groupIncKey))),
					Key:       groupIncKey,
					Reason:    active[0].reason,
					Name:      summary,
					Namespace: active[0].namespace,
					Resource:  active[0].kind,
					Resources: resources,
					PeakResources: len(resources),
					Count:     len(active),
					FirstSeen: pg.firstSeen,
					LastSeen:  now,
					Hint:      summary,
					Severity:  sev,
				}
				if mem, ok := e.state[active[0].key]; ok {
					// Carry forward rich diagnostic data from the first member
					// so the group notification includes actionable details
					// (memory limits for OOM, log signatures, etc.).
					if mem.Hint != "" && !strings.Contains(mem.Hint, summary) {
						groupInc.Hint = enricher.CombineHints(groupInc.Hint, mem.Hint)
					}
					groupInc.Logs = mem.Logs
					groupInc.IncludeLogs = mem.IncludeLogs
					groupInc.Events = mem.Events
					groupInc.IncludeEvents = mem.IncludeEvents
					groupInc.ContainerName = mem.ContainerName
					groupInc.OwnerKind = mem.OwnerKind
					groupInc.Runbook = mem.Runbook
					groupInc.Analysis = mem.Analysis
					groupInc.Image = mem.Image
					groupInc.NodeName = mem.NodeName
					groupInc.RestartCount = mem.RestartCount
					if mem.LastContainerState != nil {
						cs := *mem.LastContainerState
						groupInc.LastContainerState = &cs
					}
					groupInc.Containers = make(map[string]bool)
					for c := range mem.Containers {
						groupInc.Containers[c] = true
					}
				}
				pending = append(pending, transition{groupInc, model.ActionCreate})

				// Replace any stale tracker for the same key
				delete(e.groupMembers, gk)

				// Track group members for batch resolve
				tracker := &groupResolveTracker{
					groupIncKey: groupIncKey,
					members:     make(map[string]bool),
					totalCount:  len(active),
					summary:     summary,
					reason:      active[0].reason,
					firstSeen:   pg.firstSeen,
					lastSeen:    now,
					severity:    sev,
				}
				for _, ge := range active {
					tracker.members[ge.key] = false
				}
				e.groupMembers[gk] = tracker

				// Reset NotifiedSig on active entries so subsequent events can be re-grouped
				for _, ge := range active {
					if inc, ok := e.state[ge.key]; ok {
						inc.NotifiedSig = ""
					}
				}
			}
			delete(e.pendingGroups, gk)
			}
		}
	}

	e.mu.Unlock()

	for _, t := range pending {
		if hook := e.config.LifecycleHook; hook != nil {
			hook(t.inc, t.action)
		}
	}
	if hook := e.config.MassFailureHook; hook != nil {
		hook()
	}
	if baselineChanged {
		if hook := e.config.OnBaselineChange; hook != nil {
			e.mu.Lock()
			snapshot := cloneBaseline(e.seen)
			e.mu.Unlock()
			hook(snapshot)
		}
	}
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

// nodeLevelReasons are incident reasons that indicate a node-level problem.
// Pod-level reasons (Error, CrashLoopBackOff, etc.) should NOT be scoped
// as "node" even when all entries share the same node, because the message
// would misleadingly imply the node is the root cause.
var nodeLevelReasons = map[string]bool{
	"NodeNotReady":       true,
	"MemoryPressure":     true,
	"DiskPressure":       true,
	"PIDPressure":        true,
	"NetworkUnavailable": true,
	"NodeResourceHigh":   true,
	"NodeResourceCritical": true,
	"ContainerStatusUnknown": true,
	"Evicted":            true,
	"Preempting":         true,
}

func isNodeLevelReason(r string) bool {
	return nodeLevelReasons[r]
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

func (e *Engine) groupSeverity(entries []groupEntry) string {
	best := "normal"
	for _, ge := range entries {
		if inc, ok := e.state[ge.key]; ok {
			if r := severityRank(inc.Severity); r > severityRank(best) {
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

func (e *Engine) SetSeverityMap(m map[string]string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if en, ok := e.config.Enricher.(*enricher.DefaultEnricher); ok {
		en.SetSeverityMap(m)
	}
}
