package correlation

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/abahmed/kwatch/internal/enricher"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
	"k8s.io/klog/v2"
)

type digestEntry struct {
	key     string
	reason  string
	ns      string
}

type Config struct {
	Window            time.Duration
	Cooldown          time.Duration
	StaleThreshold    time.Duration
	LifecycleInterval time.Duration
	StartupQuiet      time.Duration
	Enricher          enricher.Enricher
	LifecycleHook     func(inc *model.Incident, action model.IncidentAction)
	BaselineTTL      time.Duration
	Baseline         map[string]int64
	OnBaselineChange  func(baseline map[string]int64)
	EscalationEnabled bool
	EscalationTiers   []int
	InhibitNodeSuppressesPods bool
	StormEnabled              bool
	StormThreshold            int
	StormWindow               time.Duration
	StormDigestInterval       time.Duration
	RenotifyInterval          time.Duration
	RenotifyIntervalBySeverity map[string]time.Duration
	RenotifyMaxPerIncident     int
}

// BuildKey constructs the incident key used for dedup, grouping, and baseline.
func BuildKey(namespace, owner, reason, container string) string {
	return namespace + ":" + owner + ":" + reason + ":" + container
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

type Engine struct {
	mu                   sync.Mutex
	state                map[string]*model.Incident
	config               Config
	startedAt            time.Time
	seen                 map[string]int64
	activeNodeIncidents  map[string]bool
	recentCreates        []time.Time
	stormUntil           time.Time
	digestBuf            []digestEntry
	lastDigestFlush      time.Time
	renotifyCount        map[string]int
	lastRenotify         map[string]time.Time
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
	e := &Engine{
		state:               make(map[string]*model.Incident),
		config:              cfg,
		startedAt:           time.Now(),
		activeNodeIncidents: make(map[string]bool),
		renotifyCount:       make(map[string]int),
		lastRenotify:        make(map[string]time.Time),
	}
	if cfg.Baseline != nil {
		e.SetSeen(cfg.Baseline)
	}
	return e
}

func (e *Engine) SetSeen(baseline map[string]int64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	now := time.Now()
	ttl := e.config.BaselineTTL
	e.seen = make(map[string]int64, len(baseline))
	for k, ts := range baseline {
		if now.Sub(time.Unix(ts, 0)) < ttl {
			e.seen[k] = ts
		}
	}
}

func (e *Engine) isBaselined(incidentKey string) bool {
	if ts, ok := e.seen[incidentKey]; ok {
		if time.Since(time.Unix(ts, 0)) < e.config.BaselineTTL {
			return true
		}
		delete(e.seen, incidentKey)
	}
	if e.config.StartupQuiet > 0 && time.Since(e.startedAt) < e.config.StartupQuiet {
		if len(e.seen) == 0 && len(e.state) == 0 {
			return true
		}
	}
	return false
}

func (e *Engine) ClearSeen(key string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.seen, key)
	if hook := e.config.OnBaselineChange; hook != nil {
		hook(cloneBaseline(e.seen))
	}
}

// ClearSeenByPrefix removes all baseline entries whose key starts with
// prefix (e.g. "ns:owner:"). Returns true if anything was removed.
func (e *Engine) ClearSeenByPrefix(prefix string) bool {
	e.mu.Lock()
	changed := false
	for k := range e.seen {
		if strings.HasPrefix(k, prefix) {
			delete(e.seen, k)
			changed = true
		}
	}
	var snapshot map[string]int64
	if changed {
		snapshot = cloneBaseline(e.seen)
	}
	e.mu.Unlock()
	if changed {
		if hook := e.config.OnBaselineChange; hook != nil {
			hook(snapshot)
		}
	}
	return changed
}

func cloneBaseline(src map[string]int64) map[string]int64 {
	dst := make(map[string]int64, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func (e *Engine) BaselineSnapshot() map[string]int64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	return cloneBaseline(e.seen)
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
}

func normalizeReason(reason string) string {
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

func (e *Engine) GetLastContainerState(namespace, podName, containerName string) *model.ContainerState {
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, inc := range e.state {
		if inc.Namespace == namespace && inc.ContainerName == containerName && inc.Resources[podName] {
			if inc.LastContainerState != nil {
				cs := *inc.LastContainerState
				return &cs
			}
			return nil
		}
	}
	return nil
}

func (e *Engine) Process(ev event.Event, owner string, cs *model.ContainerState) (*model.Incident, model.IncidentAction) {
	e.mu.Lock()
	defer e.mu.Unlock()

	r := normalizeReason(ev.Reason)

	if r == "CrashLoopBackOff" && cs != nil && cs.RestartCount > 5 {
		r = "CrashLoopHighFrequency"
	}

	key := BuildKey(ev.Namespace, owner, r, ev.ContainerName)

	if e.isBaselined(key) {
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

	now := time.Now()

	if inc, ok := e.state[key]; ok {
		if e.config.EscalationEnabled && cs != nil {
			prev := inc.RestartCount
			cur := int(cs.RestartCount)
			if t := crossedTier(prev, cur, e.config.EscalationTiers); t >= 0 {
				ev.Severity = escalateSeverity(inc.Severity)
				e.config.Enricher.Enrich(&ev, inc)
				inc.Hint = fmt.Sprintf("restart count crossed %d", e.config.EscalationTiers[t])
				inc.Count++
				inc.LastSeen = now
				inc.State = model.StateActive
				inc.LastUpdate = now
				inc.RestartCount = cur
				if ev.PodName != "" {
					inc.Resources[ev.PodName] = true
				}
				inc.LastContainerState = cs
				return inc, model.ActionUpdate
			}
		}
		if now.Before(inc.LastSeen.Add(e.config.Cooldown)) {
			return inc, model.ActionSkip
		}
		inc.Count++
		inc.LastSeen = now
		inc.State = model.StateActive
		inc.LastUpdate = now
		if ev.PodName != "" {
			inc.Resources[ev.PodName] = true
		}
		inc.LastContainerState = cs
		if cs != nil {
			inc.RestartCount = int(cs.RestartCount)
		}
		e.config.Enricher.Enrich(&ev, inc)
		return inc, model.ActionUpdate
	}

	inc := &model.Incident{
		Key:       key,
		Reason:    ev.Reason,
		Namespace: ev.Namespace,
		Resource:  res,
		Name:      owner,
		Count:     1,
		FirstSeen: now,
		LastSeen:  now,
		LastUpdate: now,
		State:     model.StateActive,
		Resources: map[string]bool{},
	}
	if ev.PodName != "" {
		inc.Resources[ev.PodName] = true
	}
	inc.LastContainerState = cs
	if cs != nil {
		inc.RestartCount = int(cs.RestartCount)
	}
	e.config.Enricher.Enrich(&ev, inc)
	e.state[key] = inc

	if e.config.StormEnabled {
		e.recentCreates = append(e.recentCreates, now)
		// prune outside window
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
			return inc, model.ActionDigest
		}
	}

	return inc, model.ActionCreate
}

func (e *Engine) MarkResolved(key string) {
	e.mu.Lock()
	inc, ok := e.state[key]
	if !ok {
		e.mu.Unlock()
		return
	}
	inc.State = model.StateResolved
	delete(e.seen, key)
	delete(e.renotifyCount, key)
	delete(e.lastRenotify, key)
	e.mu.Unlock()

	if hook := e.config.LifecycleHook; hook != nil {
		hook(inc, model.ActionResolved)
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
	for key, inc := range e.state {
		if inc.Namespace != namespace {
			continue
		}
		if !inc.Resources[podName] {
			continue
		}
			delete(inc.Resources, podName)
		if len(inc.Resources) == 0 && inc.State != model.StateResolved && inc.State != model.StateStale {
			inc.State = model.StateResolved
			delete(e.seen, key)
			delete(e.renotifyCount, key)
			delete(e.lastRenotify, key)
			baselineChanged = true
			pending = append(pending, transition{inc, model.ActionResolved})
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

func (e *Engine) ResolveByResource(resource, name string) {
	type transition struct {
		inc    *model.Incident
		action model.IncidentAction
	}
	var pending []transition
	var baselineChanged bool

	e.mu.Lock()
	if resource == "node" {
		delete(e.activeNodeIncidents, name)
	}
	for key, inc := range e.state {
		if inc.Resource == resource && inc.Name == name && inc.State != model.StateResolved && inc.State != model.StateStale {
			inc.State = model.StateResolved
			delete(e.seen, key)
			delete(e.renotifyCount, key)
			delete(e.lastRenotify, key)
			baselineChanged = true
			pending = append(pending, transition{inc, model.ActionResolved})
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
	defer e.mu.Unlock()

	now := time.Now()
	for key, inc := range e.state {
		if now.After(inc.LastSeen.Add(e.config.Window)) {
			if inc.Resource == "node" {
				delete(e.activeNodeIncidents, inc.Name)
			}
			delete(e.state, key)
			delete(e.renotifyCount, key)
			delete(e.lastRenotify, key)
		}
	}
}

func (e *Engine) checkLifecycle() {
	type transition struct {
		inc    *model.Incident
		action model.IncidentAction
	}
	var pending []transition

	e.mu.Lock()
	now := time.Now()

	// stale detection
	for _, inc := range e.state {
		if inc.State == model.StateActive && now.After(inc.LastUpdate.Add(e.config.StaleThreshold)) {
			inc.State = model.StateStale
			pending = append(pending, transition{inc, model.ActionStale})
		}
	}

	// renotify — resend stale message periodically
	if e.config.RenotifyInterval > 0 || len(e.config.RenotifyIntervalBySeverity) > 0 {
		for _, inc := range e.state {
			if inc.State != model.StateStale {
				continue
			}
			maxPer := e.config.RenotifyMaxPerIncident
			if maxPer <= 0 {
				maxPer = 3
			}
			if e.renotifyCount[inc.Key] >= maxPer {
				continue
			}
			interval := e.config.RenotifyInterval
			if len(e.config.RenotifyIntervalBySeverity) > 0 {
				if sv, ok := e.config.RenotifyIntervalBySeverity[inc.Severity]; ok && sv > 0 {
					interval = sv
				}
			}
			if interval <= 0 {
				continue
			}
			last := e.lastRenotify[inc.Key]
			if now.After(last.Add(interval)) {
				e.renotifyCount[inc.Key]++
				e.lastRenotify[inc.Key] = now
				pending = append(pending, transition{inc, model.ActionStale})
			}
		}
	}

	// digest flush
	if e.config.StormEnabled && len(e.digestBuf) > 0 && now.After(e.lastDigestFlush.Add(e.config.StormDigestInterval)) {
		summary := e.buildDigestSummary()
		n := len(e.digestBuf)
		e.digestBuf = nil
		e.lastDigestFlush = now
		digestInc := &model.Incident{
			Key:    "digest:" + strconv.FormatInt(now.Unix(), 10),
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
}

func (e *Engine) buildDigestSummary() string {
	if len(e.digestBuf) == 0 {
		return ""
	}
	byReason := make(map[string]map[string]int) // reason → ns → count
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
