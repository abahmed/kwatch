package alert

import (
	"context"
	"reflect"
	"strings"
	"sync"
	"text/template"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/event"
	"github.com/abahmed/kwatch/internal/insight"
	"github.com/abahmed/kwatch/internal/model"
)

type providerEntry struct {
	provider      Provider
	routes        []config.AlertRoute
	retry         retryConfig
	fallback      *providerEntry
	fallbackNamed string // resolved in second pass
	templates     map[string]*template.Template
	maxBytes      int // 0 = no limit (FIX-5)
	ch            chan deliverJob
}

type AlertManager struct {
	entries     []providerEntry
	silences    []silenceMatcher
	maxLogLines int
	templates   map[string]*template.Template
	clusterName string
	started     bool
	stopped     bool
	mu          sync.Mutex
	cfgMu       sync.RWMutex
	providerWg  sync.WaitGroup
	dlqMu       sync.Mutex
	dlqRing     [dlqCap]DeadLetterEntry
	dlqHead     int

	done chan struct{}

	ctx context.Context
}

func (a *AlertManager) SetMaxLogLines(n int) {
	if n > 0 {
		a.cfgMu.Lock()
		a.maxLogLines = n
		a.cfgMu.Unlock()
	}
}

func (a *AlertManager) SetTemplates(tpl map[string]string) {
	if len(tpl) == 0 {
		a.templates = nil
		return
	}
	a.templates = make(map[string]*template.Template, len(tpl))
	for reason, raw := range tpl {
		t, err := template.New(reason).Option("missingkey=zero").Parse(raw)
		if err != nil {
			klog.ErrorS(err, "invalid template, skipping", "reason", reason)
			continue
		}
		a.templates[strings.ToLower(reason)] = t
	}
}

type Provider interface {
	Name() string
	SendEvent(*event.Event) error
	SendMessage(string) error
}

// VerifiableProvider is an optional interface for providers that support
// credential pre-flight verification (kwatch lint --check).

type VerifiableProvider interface {
	Verify() error
}

func (a *AlertManager) Init(
	alertCfg map[string]map[string]interface{},
	appCfg *config.App,
) {
	if a.started {
		a.shutdown()
	}
	a.entries = make([]providerEntry, 0)
	a.silences = nil
	if appCfg != nil {
		a.clusterName = appCfg.ClusterName
	}

	entries := make([]providerEntry, 0, len(alertCfg))
	for k, v := range alertCfg {
		lowerCaseKey := strings.ToLower(k)
		pvdr := newProvider(lowerCaseKey, v, appCfg)
		if pvdr == nil {
			if config.KnownProviders[lowerCaseKey] {
				klog.InfoS(
					"alert provider has missing or invalid credentials, "+
						"skipping",
					"name",
					k,
				)
			} else {
				klog.InfoS("unknown alert provider, skipping", "name", k)
			}
			continue
		}
		if !reflect.ValueOf(pvdr).IsNil() {
			rc := extractRetry(v)
			fbName := ""
			if raw, ok := v["fallback"]; ok {
				fbName, _ = raw.(string)
			}
			entries = append(entries, providerEntry{
				provider:      pvdr,
				routes:        extractRoutes(v),
				retry:         rc,
				fallback:      nil,
				fallbackNamed: fbName,
				templates:     extractTemplates(v),
				maxBytes:      defaultMaxBytes(pvdr.Name()),
				ch:            make(chan deliverJob, channelCap),
			})
		}
	}
	// second pass: resolve fallback names to pointers
	for i := range entries {
		if entries[i].fallbackNamed != "" {
			for j := range entries {
				if strings.EqualFold(
					entries[j].provider.Name(),
					entries[i].fallbackNamed,
				) {
					entries[i].fallback = &entries[j]
					break
				}
			}
			if entries[i].fallback == nil {
				klog.InfoS(
					"fallback provider not found, skipping",
					"provider",
					entries[i].provider.Name(),
					"fallback",
					entries[i].fallbackNamed,
				)
			}
			entries[i].fallbackNamed = ""
		}
	}
	a.entries = entries
}

// SetSilences configures silence rules on the alert manager.
// Must be called after Init.

func (a *AlertManager) VerifyAll() map[string]error {
	result := make(map[string]error)
	for _, entry := range a.entries {
		if v, ok := entry.provider.(VerifiableProvider); ok {
			result[entry.provider.Name()] = v.Verify()
		} else {
			result[entry.provider.Name()] = nil // no verifier = skip
		}
	}
	return result
}

// Notify sends string msg to all providers

func (a *AlertManager) Notify(msg string) {
	klog.InfoS("sending message", "msg", msg)

	a.mu.Lock()
	entries := make([]providerEntry, len(a.entries))
	copy(entries, a.entries)
	a.mu.Unlock()

	for _, entry := range entries {
		p := entry.provider
		ctx := a.ctx
		if ctx == nil {
			ctx = context.Background()
		}
		if _, ok := p.(EventDeliveryProvider); ok {
			ev := &event.Event{
				PodName: msg,
				Reason:  constant.ReasonNotify,
			}
			if err := sendWithRetry(ctx, func() error {
				return p.SendEvent(ev)
			}, entry.retry, p.Name()); err != nil && entry.fallback != nil {
				if fbErr := entry.fallback.provider.SendMessage(
					"[fallback — primary " + p.Name() + " failed] " + msg,
				); fbErr != nil {
					klog.ErrorS(
						fbErr,
						"fallback provider failed",
						"provider",
						entry.fallback.provider.Name(),
					)
				}
			}
			continue
		}
		truncMsg := truncateMsg(msg, entry.maxBytes)
		if err := sendWithRetry(ctx, func() error {
			return p.SendMessage(truncMsg)
		}, entry.retry, p.Name()); err != nil && entry.fallback != nil {
			fbMsg := truncateMsg(
				"[fallback — primary "+p.Name()+" failed] "+truncMsg,
				entry.fallback.maxBytes,
			)
			if fbErr := entry.fallback.provider.SendMessage(
				fbMsg,
			); fbErr != nil {
				klog.ErrorS(
					fbErr,
					"fallback provider failed",
					"provider",
					entry.fallback.provider.Name(),
				)
			}
		}
	}
}

// NotifyEvent sends event to all providers

func (a *AlertManager) NotifyEvent(event event.Event) {
	klog.InfoS("sending event", "event", event)

	a.mu.Lock()
	entries := make([]providerEntry, len(a.entries))
	copy(entries, a.entries)
	a.mu.Unlock()

	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	for _, entry := range entries {
		p := entry.provider
		if err := sendWithRetry(ctx, func() error {
			return p.SendEvent(&event)
		}, entry.retry, p.Name()); err != nil && entry.fallback != nil {
			if ferr := entry.fallback.provider.SendMessage(
				"[fallback — primary " + p.Name() + " failed] " + event.Reason +
					" in " + event.Namespace + "/" + event.PodName,
			); ferr != nil {
				klog.ErrorS(
					ferr,
					"fallback provider send failed",
					"primary",
					p.Name(),
					"fallback",
					entry.fallback.provider.Name(),
				)
			}
		}
	}
}

// ThreadProvider is an optional interface for providers that support
// incident-aware messaging (e.g., Slack threads).

type ThreadProvider interface {
	SendIncident(inc *model.Incident, action model.IncidentAction) error
}

// InsightThreadProvider is a ThreadProvider that can also show the insight
// engine's diagnosis — likely cause, impact, recent changes — in its own
// format. Without it a rich provider builds its message from the incident
// alone and the diagnosis is silently dropped.
type InsightThreadProvider interface {
	ThreadProvider
	SendIncidentWithInsight(
		inc *model.Incident,
		action model.IncidentAction,
		ins *insight.Insight,
	) error
}

// EventDeliveryProvider is a marker interface for providers whose real
// delivery is implemented in SendEvent (not SendMessage). PagerDuty,
// Opsgenie, Zenduty, and Email all stub SendMessage to return nil — the
// routing layer must call SendEvent instead for these providers.

type EventDeliveryProvider interface {
	Provider
	UsesEventDelivery()
}

// incidentToEvent maps a delivered incident to the legacy event.Event shape
// these EventDeliveryProvider providers' SendEvent expects.

func (a *AlertManager) NotifyIncident(
	inc *model.Incident,
	action model.IncidentAction,
	insight *insight.Insight,
) {
	if action == model.ActionSkip {
		return
	}

	if a.isSilenced(inc) {
		klog.V(4).InfoS("incident suppressed by silence rule",
			"key", inc.Key, "id", inc.ID, "reason", inc.Reason,
			"namespace", inc.Namespace)
		return
	}

	klog.InfoS(
		"sending incident",
		"action",
		action,
		"key",
		inc.Key,
		"id",
		inc.ID,
		"count",
		inc.Count,
	)

	if !a.started {
		a.deliverAllSync(inc, action, insight)
		return
	}

	snap := inc.Clone()
	ins := insight
	if ins != nil {
		cp := *ins
		ins = &cp
	}
	job := deliverJob{inc: snap, action: action, insight: ins}

	a.mu.Lock()
	stopped := a.stopped
	if !stopped {
		a.fanOut(job)
	}
	a.mu.Unlock()
}

// AddProvider appends a provider entry for testing or late registration.

func (a *AlertManager) AddProvider(p Provider) {
	a.mu.Lock()
	defer a.mu.Unlock()
	entry := providerEntry{
		provider: p,
		retry: retryConfig{
			maxAttempts: 1,
			delay:       time.Second,
			maxBackoff:  defaultMaxBackoff,
		},
		ch: make(chan deliverJob, channelCap),
	}
	a.entries = append(a.entries, entry)
	if a.started {
		a.providerWg.Add(1)
		go func() {
			defer a.providerWg.Done()
			for job := range entry.ch {
				a.deliverOne(a.ctx, &entry, job.inc, job.action, job.insight)
			}
		}()
	}
}

// Start launches a worker goroutine for each provider that processes
// queued deliveries. Workers drain and stop when ctx is cancelled.

func (a *AlertManager) Start(ctx context.Context) {
	a.mu.Lock()
	a.started = true
	a.stopped = false
	a.ctx = ctx
	a.done = make(chan struct{})
	entries := make([]providerEntry, len(a.entries))
	copy(entries, a.entries)
	a.mu.Unlock()

	for i := range entries {
		entry := &entries[i]
		a.providerWg.Add(1)
		go func() {
			defer a.providerWg.Done()
			for job := range entry.ch {
				a.deliverOne(a.ctx, entry, job.inc, job.action, job.insight)
			}
		}()
	}
	go func() {
		<-ctx.Done()
		a.shutdown()
	}()
}

// shutdown waits for all delivery workers to finish (used in tests).

func (a *AlertManager) shutdown() {
	a.mu.Lock()
	if a.stopped {
		a.mu.Unlock()
		return
	}
	a.stopped = true
	entries := make([]providerEntry, len(a.entries))
	copy(entries, a.entries)
	a.mu.Unlock()

	// 1) close provider channels under a.mu so fanOut (also under a.mu) never
	//    sends on a closed channel.
	a.mu.Lock()
	for i := range entries {
		if entries[i].ch != nil {
			close(entries[i].ch)
		}
	}
	a.mu.Unlock()
	a.providerWg.Wait()
	close(a.done)
}

// Done returns a channel that is closed when the AlertManager has fully
// drained and shut down (all provider workers finished).

func (a *AlertManager) Done() <-chan struct{} {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.done != nil {
		return a.done
	}
	ch := make(chan struct{})
	close(ch)
	return ch
}

// DeadLetters returns a copy of the dead-letter ring buffer.

func (a *AlertManager) DeadLetters() interface{} {
	a.dlqMu.Lock()
	defer a.dlqMu.Unlock()
	n := 0
	for i := range a.dlqRing {
		if a.dlqRing[i].Timestamp.IsZero() {
			break
		}
		n++
	}
	out := make([]DeadLetterEntry, n)
	for i := 0; i < n; i++ {
		idx := (a.dlqHead - n + i + dlqCap) % dlqCap
		out[i] = a.dlqRing[idx]
	}
	return out
}
