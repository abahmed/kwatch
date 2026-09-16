package delivery

import (
	"context"
	"reflect"
	"strings"
	"sync"
	"text/template"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/config"
	deliveryapi "github.com/abahmed/kwatch/internal/delivery/api"
	"github.com/abahmed/kwatch/internal/delivery/transport"
)

type providerEntry struct {
	provider     Provider
	routes       []config.AlertRoute
	retry        retryConfig
	fallbackName string
	templates    map[string]*template.Template
	maxBytes     int // 0 = no limit (FIX-5)
	ch           chan deliverJob
}

// providerGeneration is an immutable lookup snapshot for one configured
// provider set. Entries are values so a reconfiguration cannot invalidate a
// fallback while an older delivery is still finishing.
type providerGeneration struct {
	entries map[string]providerEntry
	order   []string
}

type Manager struct {
	generation   *providerGeneration
	silences     []silenceMatcher
	templates    map[string]*template.Template
	clusterName  string
	providerDeps transport.Dependencies
	started      bool
	stopped      bool
	mu           sync.Mutex
	cfgMu        sync.RWMutex
	workerCount  int
	workerDone   chan struct{}
	workerCtx    context.Context
	cancelWorker context.CancelFunc
	dlqMu        sync.Mutex
	dlqRing      [dlqCap]DeadLetterEntry
	dlqHead      int
	dlqCount     int

	done chan struct{}

	ctx context.Context
	now func() time.Time

	// pacer spreads deliveries per provider and holds the overflow digests.
	pacer sendPacer
}

func (a *Manager) nowTime() time.Time {
	if a.now == nil {
		return time.Time{}
	}
	return a.now()
}

func (a *Manager) globalTemplates() map[string]*template.Template {
	a.cfgMu.RLock()
	defer a.cfgMu.RUnlock()
	return a.templates
}

// Provider is the canonical cancellation-aware provider contract.
type Provider = deliveryapi.Provider

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

// Verifier is an optional interface for providers that support credential
// pre-flight verification (kwatch lint --check).
type Verifier interface {
	Verify(context.Context) error
}

// ProviderFactory constructs a configured provider. The application supplies
// the static catalog so delivery owns dispatch behavior, not provider wiring.
type ProviderFactory func(
	string,
	map[string]interface{},
	transport.ProviderContext,
) Provider

// InitRuntime initializes delivery from the immutable configuration snapshot.
// Production composition uses this path so routing and retry policy are not
// reparsed from the YAML-facing map.
func (a *Manager) InitRuntime(
	runtime config.RuntimeConfig,
	factory ProviderFactory,
) {
	a.initRuntime(runtime, factory)
}

func (a *Manager) initRuntime(
	runtime config.RuntimeConfig,
	factory ProviderFactory,
) {
	a.mu.Lock()
	active := a.started && !a.stopped
	activeContext := a.ctx
	a.mu.Unlock()
	if active {
		a.shutdown()
	}
	clusterName := runtime.Application().ClusterName
	providerContext := transport.ProviderContext{
		ClusterName:  clusterName,
		Dependencies: a.providerDeps,
	}

	providers := runtime.Providers()
	entries := make([]providerEntry, 0, len(providers))
	for _, provider := range providers {
		lowerCaseKey := strings.ToLower(provider.Name)
		var pvdr Provider
		if factory != nil {
			pvdr = factory(lowerCaseKey, provider.Settings, providerContext)
		}
		if pvdr == nil {
			if config.IsKnownProvider(lowerCaseKey) {
				klog.InfoS(
					"alert provider has missing or invalid credentials, "+
						"skipping",
					"name",
					provider.Name,
				)
			} else {
				klog.InfoS(
					"unknown alert provider, skipping", "name", provider.Name,
				)
			}
			continue
		}
		if !isNilProvider(pvdr) {
			entries = append(entries, providerEntry{
				provider:     pvdr,
				routes:       provider.Routes,
				retry:        retryConfigFromRuntime(provider.Retry),
				fallbackName: provider.FallbackName,
				templates:    compileTemplates(provider.Templates),
				maxBytes:     defaultMaxBytes(pvdr.Name()),
				ch:           make(chan deliverJob, channelCap),
			})
		}
	}
	// Validate fallback names after all providers are initialized. Production
	// dispatch resolves names against the current immutable generation.
	for i := range entries {
		if entries[i].fallbackName != "" {
			found := false
			for j := range entries {
				if strings.EqualFold(
					entries[j].provider.Name(),
					entries[i].fallbackName,
				) {
					found = true
					break
				}
			}
			if !found {
				klog.InfoS(
					"fallback provider not found, skipping",
					"provider",
					entries[i].provider.Name(),
					"fallback",
					entries[i].fallbackName,
				)
			}
		}
	}
	for _, providerName := range sanitizeFallbackCycles(entries) {
		klog.InfoS(
			"fallback cycle detected; disabling fallback",
			"provider", providerName,
		)
	}
	a.mu.Lock()
	a.generation = newProviderGeneration(entries)
	a.pacer = sendPacer{}
	a.clusterName = clusterName
	a.started = false
	a.stopped = false
	a.ctx = nil
	a.done = nil
	a.mu.Unlock()
	a.cfgMu.Lock()
	a.silences = compileSilences(runtime.Silences())
	a.templates = compileTemplates(runtime.Templates())
	a.cfgMu.Unlock()
	if active {
		if err := a.Start(activeContext); err != nil {
			klog.ErrorS(err, "failed to restart delivery workers")
		}
	}
}

func newProviderGeneration(entries []providerEntry) *providerGeneration {
	generation := &providerGeneration{
		entries: make(map[string]providerEntry, len(entries)),
		order:   make([]string, 0, len(entries)),
	}
	for _, entry := range entries {
		name := strings.ToLower(entry.provider.Name())
		generation.entries[name] = entry
		generation.order = append(generation.order, name)
	}
	return generation
}

func (a *Manager) currentGenerationLocked() *providerGeneration {
	return a.generation
}

func (a *Manager) VerifyAll(ctx context.Context) map[string]error {
	result := make(map[string]error)
	a.mu.Lock()
	generation := a.currentGenerationLocked()
	providers := make([]Provider, 0)
	if generation != nil {
		providers = make([]Provider, 0, len(generation.order))
		for _, name := range generation.order {
			providers = append(providers,
				generation.entries[name].provider)
		}
	}
	a.mu.Unlock()
	for _, provider := range providers {
		if v, ok := provider.(Verifier); ok {
			result[provider.Name()] = v.Verify(ctx)
		} else {
			result[provider.Name()] = nil // no verifier = skip
		}
	}
	return result
}
