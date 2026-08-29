package alert

import (
	"context"
	"reflect"
	"strings"
	"sync"
	"text/template"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/event"
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

func (a *AlertManager) SetTemplates(tpl map[string]string) {
	if len(tpl) == 0 {
		a.cfgMu.Lock()
		a.templates = nil
		a.cfgMu.Unlock()
		return
	}
	templates := make(map[string]*template.Template, len(tpl))
	for reason, raw := range tpl {
		t, err := template.New(reason).Option("missingkey=zero").Parse(raw)
		if err != nil {
			klog.ErrorS(err, "invalid template, skipping", "reason", reason)
			continue
		}
		templates[strings.ToLower(reason)] = t
	}
	a.cfgMu.Lock()
	a.templates = templates
	a.cfgMu.Unlock()
}

func (a *AlertManager) globalTemplates() map[string]*template.Template {
	a.cfgMu.RLock()
	defer a.cfgMu.RUnlock()
	return a.templates
}

type Provider interface {
	Name() string
	SendEvent(*event.Event) error
	SendMessage(string) error
}

func isNilProvider(p Provider) bool {
	if p == nil {
		return true
	}
	v := reflect.ValueOf(p)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map,
		reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
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
	if a.started && !a.stopped {
		a.shutdown()
	}
	if appCfg == nil {
		appCfg = &config.App{}
	}
	a.entries = make([]providerEntry, 0)
	a.silences = nil
	a.clusterName = appCfg.ClusterName

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
		if !isNilProvider(pvdr) {
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
	a.mu.Lock()
	a.entries = entries
	a.started = false
	a.stopped = false
	a.ctx = nil
	a.done = nil
	a.mu.Unlock()
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
