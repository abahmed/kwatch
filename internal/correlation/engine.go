package correlation

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/abahmed/kwatch/internal/enricher"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/model"
	"k8s.io/klog/v2"
)

type Config struct {
	Window            time.Duration
	Cooldown          time.Duration
	StaleThreshold    time.Duration
	LifecycleInterval time.Duration
	StartupQuiet      time.Duration
	Enricher          enricher.Enricher
	LifecycleHook     func(inc *model.Incident, action model.IncidentAction)
}

type Engine struct {
	mu          sync.Mutex
	state       map[string]*model.Incident
	config      Config
	startedAt   time.Time
	seen        map[string]bool
}

func NewEngine(cfg Config) *Engine {
	if cfg.Enricher == nil {
		cfg.Enricher = &enricher.DefaultEnricher{}
	}
	if cfg.LifecycleInterval <= 0 {
		cfg.LifecycleInterval = 1 * time.Minute
	}
	return &Engine{
		state:     make(map[string]*model.Incident),
		config:    cfg,
		startedAt: time.Now(),
	}
}

func (e *Engine) SetSeen(keys []string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.seen = make(map[string]bool, len(keys))
	for _, k := range keys {
		e.seen[k] = true
	}
}

func (e *Engine) isBaselined(podKey string) bool {
	if len(e.seen) > 0 && e.seen[podKey] {
		return true
	}
	if e.config.StartupQuiet > 0 && time.Since(e.startedAt) < e.config.StartupQuiet {
		if len(e.seen) == 0 {
			return true
		}
	}
	return false
}

func (e *Engine) ClearSeen(podKey string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.seen, podKey)
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

	podKey := ev.Namespace + "/" + ev.PodName
	if e.isBaselined(podKey) {
		return nil, model.ActionSkip
	}

	r := normalizeReason(ev.Reason)

	if r == "CrashLoopBackOff" && cs != nil && cs.RestartCount > 5 {
		r = "CrashLoopHighFrequency"
	}

	key := ev.Namespace + ":" + owner + ":" + r + ":" + ev.ContainerName
	now := time.Now()

	if inc, ok := e.state[key]; ok {
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
		e.config.Enricher.Enrich(&ev, inc)
		return inc, model.ActionUpdate
	}

	res := ev.Resource
	if res == "" {
		res = "pod"
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
	e.config.Enricher.Enrich(&ev, inc)
	e.state[key] = inc
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
	e.mu.Unlock()

	if hook := e.config.LifecycleHook; hook != nil {
		hook(inc, model.ActionResolved)
	}
}

func (e *Engine) RemovePod(namespace, podName string) {
	type transition struct {
		inc    *model.Incident
		action model.IncidentAction
	}
	var pending []transition

	e.mu.Lock()
	for _, inc := range e.state {
		if inc.Namespace != namespace {
			continue
		}
		if !inc.Resources[podName] {
			continue
		}
		delete(inc.Resources, podName)
		if len(inc.Resources) == 0 && inc.State != model.StateResolved && inc.State != model.StateStale {
			inc.State = model.StateResolved
			pending = append(pending, transition{inc, model.ActionResolved})
		}
	}
	delete(e.seen, namespace+"/"+podName)
	e.mu.Unlock()

	for _, t := range pending {
		if hook := e.config.LifecycleHook; hook != nil {
			hook(t.inc, t.action)
		}
	}
}

func (e *Engine) ResolveByResource(resource, name string) {
	type transition struct {
		inc    *model.Incident
		action model.IncidentAction
	}
	var pending []transition

	e.mu.Lock()
	for _, inc := range e.state {
		if inc.Resource == resource && inc.Name == name && inc.State != model.StateResolved && inc.State != model.StateStale {
			inc.State = model.StateResolved
			pending = append(pending, transition{inc, model.ActionResolved})
		}
	}
	e.mu.Unlock()

	for _, t := range pending {
		if hook := e.config.LifecycleHook; hook != nil {
			hook(t.inc, t.action)
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
			delete(e.state, key)
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
	for _, inc := range e.state {
		if inc.State == model.StateActive && now.After(inc.LastUpdate.Add(e.config.StaleThreshold)) {
			inc.State = model.StateStale
			pending = append(pending, transition{inc, model.ActionStale})
		}
	}
	e.mu.Unlock()

	for _, t := range pending {
		if hook := e.config.LifecycleHook; hook != nil {
			hook(t.inc, t.action)
		}
	}
}
