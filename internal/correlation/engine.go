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

	"github.com/abahmed/kwatch/internal/enricher"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/metrics"
	"github.com/abahmed/kwatch/internal/model"
	"k8s.io/klog/v2"
)

type digestEntry struct {
	key    string
	reason string
	ns     string
}

type Config struct {
	Window                     time.Duration
	LifecycleInterval          time.Duration
	Enricher                   enricher.Enricher
	LifecycleHook              func(inc *model.Incident, action model.IncidentAction)
	BaselineTTL                time.Duration
	Baseline                   map[string]map[string]int64
	OnBaselineChange           func(baseline map[string]map[string]int64)
	EscalationEnabled          bool
	EscalationTiers            []int
	InhibitNodeSuppressesPods  bool
	StormEnabled               bool
	StormThreshold             int
	StormWindow                time.Duration
	StormDigestInterval        time.Duration
	MaxBaseline                int
	RenotifyIntervalBySeverity map[string]time.Duration
	RenotifyMaxPerIncident     int
	ResolveHoldDown            time.Duration
	Runbooks                   map[string]string
}

// BuildKey constructs the incident key used for dedup, grouping, and baseline.
func BuildKey(namespace, owner, reason, container string) string {
	return namespace + ":" + owner + ":" + reason + ":" + container
}

// IncidentKey derives a dedup key from an event, mirroring the exact normalisation
// chain inside Process. It returns the same key that Process would compute.
func IncidentKey(ev event.Event, owner string, cs *model.ContainerState) string {
	r := normalizeReason(ev.Reason)
	if r == "CrashLoopBackOff" && cs != nil && cs.RestartCount > defaultCrashLoopHighFreqThreshold {
		r = "CrashLoopHighFrequency"
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
	// Suppress resolve for incidents that were only ever digested (operator never saw the create)
	if inc.Digested && inc.State == model.StateResolved {
		inc.NotifiedSig = notifSig(inc)
		inc.LastNotifiedAt = e.now()
		return model.ActionSkip
	}
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
	inc.Digested = false // real create/update edge firing → operator now has visibility; allow future renotify and resolve
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

// escalateSeverity moves severity one level up: "" → "high", "high" → "critical", "critical" → "critical".
func escalateSeverity(s string) string {
	switch s {
	case "", "normal":
		return "high"
	case "high":
		return "critical"
	default:
		return s
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
	activeNodeIncidents map[string]bool
	lastContainerIndex  map[string]*model.ContainerState // key: namespace/podName
	recentCreates       []time.Time
	stormUntil          time.Time
	digestBuf           []digestEntry
	lastDigestFlush     time.Time
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
	now := e.now()
	ttl := e.config.BaselineTTL
	e.seen = make(map[string]map[string]int64, len(b))
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

// ClearSeenForPod removes all baseline entries for the given pod.
func (e *Engine) ClearSeenForPod(namespace, podName string) {
	e.mu.Lock()
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

func (e *Engine) BaselineSnapshot() map[string]map[string]int64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	return cloneBaseline(e.seen)
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
	// Normalize ErrImagePull → ImagePullBackOff (same root cause)
	if reason == "ErrImagePull" {
		return "ImagePullBackOff"
	}
	idx := strings.LastIndex(reason, " ")
	if idx > 0 {
		base, suffix := reason[:idx], reason[idx+1:]
		if _, err := strconv.Atoi(suffix); err == nil && knownRetryReasons[base] {
			return base
		}
	}
	return reason
}

func (e *Engine) findNodeIncident(nodeName string) *model.Incident {
	for _, inc := range e.state {
		if inc.Resource == "node" && inc.Name == nodeName {
			return inc
		}
	}
	return nil
}

// CountActiveNodeIncidents returns the number of nodes with active
// (non-resolved) incidents. Used for node→resource inhibition decisions.
func (e *Engine) CountActiveNodeIncidents() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.activeNodeIncidents)
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

// GetIncidentsByNamespace returns incident views filtered to a single namespace.
func (e *Engine) GetIncidentsByNamespace(ns string) []model.IncidentView {
	e.mu.Lock()
	defer e.mu.Unlock()
	byNS := e.namespaceIndex[ns]
	out := make([]model.IncidentView, 0, len(byNS))
	for _, inc := range byNS {
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

func (e *Engine) indexLastContainerState(namespace, podName string, cs *model.ContainerState) {
	if podName == "" || cs == nil {
		return
	}
	cp := *cs
	e.lastContainerIndex[namespace+"/"+podName] = &cp
}

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

func (e *Engine) Process(ev event.Event, owner string, cs *model.ContainerState) (incident *model.Incident, action model.IncidentAction) {
	e.mu.Lock()
	defer e.mu.Unlock()
	defer func() {
		if incident != nil {
			incident = incident.Clone()
		}
	}()

	key := IncidentKey(ev, owner, cs)

	if e.isBaselined(key, ev.PodName) {
		return nil, model.ActionSkip
	}

	res := ev.Resource
	if res == "" {
		res = "pod"
	}

	// Track active node incidents for pod suppression
	if res == "node" && ev.NodeName != "" {
		e.activeNodeIncidents[ev.NodeName] = true
	}

	// Suppress pod incidents when the node has an active incident
	if e.config.InhibitNodeSuppressesPods &&
		res == "pod" && ev.NodeName != "" && e.activeNodeIncidents[ev.NodeName] {
		if nodeInc := e.findNodeIncident(ev.NodeName); nodeInc != nil {
			nodeInc.SuppressedPods++
		}
		return nil, model.ActionSkip
	}

	now := e.now()

	if inc, ok := e.state[key]; ok {
		// Already resolved — re-create as fresh incident
		if inc.State == model.StateResolved {
			newInc := e.newIncident(ev, owner, cs, key, res, now)
			e.state[key] = newInc
			e.indexIncidentByNamespace(newInc)
			return newInc, e.edgeAction(newInc)
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
		return inc, e.edgeAction(inc)
	}

	inc := e.newIncident(ev, owner, cs, key, res, now)
	e.state[key] = inc
	e.indexIncidentByNamespace(inc)

	if e.config.StormEnabled {
		e.recentCreates = append(e.recentCreates, now)
		cutoff := now.Add(-e.config.StormWindow)
		kept := 0
		for _, t := range e.recentCreates {
			if t.After(cutoff) {
				e.recentCreates[kept] = t
				kept++
			}
		}
		e.recentCreates = e.recentCreates[:kept]

		if now.Before(e.stormUntil) || len(e.recentCreates) >= e.config.StormThreshold {
			e.stormUntil = now.Add(e.config.StormWindow)
			e.digestBuf = append(e.digestBuf, digestEntry{key: key, reason: ev.Reason, ns: ev.Namespace})
			inc.Digested = true
			inc.NotifiedSig = notifSig(inc)
			inc.LastNotifiedAt = now
			metrics.Default.IncidentsDigest.Add(1)
			return inc, model.ActionDigest
		}
	}

	return inc, e.edgeAction(inc)
}

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
	return inc
}

func (e *Engine) MarkResolved(key string) {
	e.mu.Lock()
	inc, ok := e.state[key]
	if !ok || inc.State == model.StateResolved || inc.State == model.StatePendingResolve {
		e.mu.Unlock()
		return
	}
	if e.config.ResolveHoldDown > 0 {
		inc.State = model.StatePendingResolve
		inc.ResolveAt = e.now().Add(e.config.ResolveHoldDown)
		e.mu.Unlock()
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
	type transition struct {
		inc    *model.Incident
		action model.IncidentAction
	}
	var pending []transition
	var baselineChanged bool

	e.mu.Lock()
	now := e.now()
	for key, inc := range e.state {
		if inc.Namespace != namespace {
			continue
		}
		if !inc.Resources[podName] {
			continue
		}
		delete(inc.Resources, podName)
		if len(inc.Resources) == 0 && inc.State != model.StateResolved {
			if inc.State == model.StatePendingResolve {
				continue
			}
			if e.config.ResolveHoldDown > 0 {
				inc.State = model.StatePendingResolve
				inc.ResolveAt = now.Add(e.config.ResolveHoldDown)
				continue
			}
			inc.State = model.StateResolved
			delete(e.seen, key)
			action := e.edgeAction(inc)
			baselineChanged = true
			pending = append(pending, transition{inc.Clone(), action})
		}
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

func (e *Engine) ResolveByResource(resource, name string) {
	type transition struct {
		inc    *model.Incident
		action model.IncidentAction
	}
	var pending []transition
	var baselineChanged bool

	e.mu.Lock()
	now := e.now()
	for key, inc := range e.state {
		if inc.Resource == resource && inc.Name == name && inc.State != model.StateResolved {
			if inc.State == model.StatePendingResolve {
				continue
			}
			if e.config.ResolveHoldDown > 0 {
				inc.State = model.StatePendingResolve
				inc.ResolveAt = now.Add(e.config.ResolveHoldDown)
				continue
			}
			inc.State = model.StateResolved
			if inc.Resource == "node" {
				e.refreshNodeInhibition(inc.Name)
			}
			delete(e.seen, key)
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
		// Finalize active/digested incidents with a resolve so the
		// LifecycleHook emits a resolved notification and Slack's
		// threadMap is pruned. Skip StatePendingResolve — that state is
		// owned by checkLifecycle.
		if inc.State != model.StateResolved && inc.State != model.StatePendingResolve {
			inc.State = model.StateResolved
			if a := e.edgeAction(inc); a != model.ActionSkip {
				pending = append(pending, transition{inc.Clone(), a})
			}
		}
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
	now := e.now()

	// pending resolve finalization
	for key, inc := range e.state {
		if inc.State == model.StatePendingResolve && !inc.ResolveAt.IsZero() && now.After(inc.ResolveAt) {
			inc.State = model.StateResolved
			if inc.Resource == "node" {
				e.refreshNodeInhibition(inc.Name)
			}
			delete(e.seen, key)
			action := e.edgeAction(inc)
			baselineChanged = true
			pending = append(pending, transition{inc.Clone(), action})
		}
	}

	// renotify — resend on time-based interval (not stale-gated)
	renotifyBySev := e.config.RenotifyIntervalBySeverity
	if len(renotifyBySev) > 0 {
		for _, inc := range e.state {
			if inc.State == model.StateResolved || inc.State == model.StatePendingResolve || inc.Digested {
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

	// digest flush
	if e.config.StormEnabled && len(e.digestBuf) > 0 && now.After(e.lastDigestFlush.Add(e.config.StormDigestInterval)) {
		n := len(e.digestBuf)
		summary := e.buildDigestSummary()
		e.digestBuf = nil
		e.lastDigestFlush = now
		if summary == "" {
			summary = fmt.Sprintf("⚡ %d new incident(s) during storm window (%s)",
				n, e.config.StormWindow.String())
		}
		digestKey := "digest:" + strconv.FormatInt(now.Unix(), 10)
		digestInc := &model.Incident{
			ID:     fmt.Sprintf("%08x", crc32.ChecksumIEEE([]byte(digestKey))),
			Key:    digestKey,
			Reason: "DigestSummary",
			Count:  n,
			Hint:   summary,
		}
		pending = append(pending, transition{digestInc, model.ActionDigestFlush})
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

func (e *Engine) buildDigestSummary() string {
	if len(e.digestBuf) == 0 {
		return ""
	}
	byReason := make(map[string]map[string]int)
	for _, d := range e.digestBuf {
		if byReason[d.reason] == nil {
			byReason[d.reason] = make(map[string]int)
		}
		byReason[d.reason][d.ns]++
	}
	var parts []string
	for reason, nsMap := range byReason {
		total := 0
		for _, c := range nsMap {
			total += c
		}
		if total <= 1 {
			continue
		}
		nsList := make([]string, 0, len(nsMap))
		for ns := range nsMap {
			nsList = append(nsList, ns)
		}
		sort.Strings(nsList)
		parts = append(parts, fmt.Sprintf("%s × %d (in %s)", reason, total, strings.Join(nsList, ", ")))
	}
	if len(parts) == 0 {
		return ""
	}
	sort.Strings(parts)
	var top []string
	if len(parts) > 5 {
		top = parts[:5]
	} else {
		top = parts
	}
	window := e.config.StormWindow
	return fmt.Sprintf("⚡ %d new incidents in %s — %s", len(e.digestBuf), window.String(), strings.Join(top, "; "))
}

func (e *Engine) SetSeverityMap(m map[string]string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if en, ok := e.config.Enricher.(*enricher.DefaultEnricher); ok {
		en.SetSeverityMap(m)
	}
}
