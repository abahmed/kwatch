package delivery

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"text/template"
	"time"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/config"
	deliveryapi "github.com/abahmed/kwatch/internal/delivery/api"
	"github.com/abahmed/kwatch/internal/delivery/transport"
)

type providerEntry struct {
	catalogName  string
	provider     Provider
	routes       []config.AlertRoute
	retry        retryConfig
	fallbackName string
	templates    map[string]*template.Template
	maxBytes     int // 0 = no limit (FIX-5)
	ch           chan deliverJob
}

type generationState string

// ErrNoReconfiguration means the manager completion belonged to shutdown or
// an unexpected worker stop rather than a generation replacement.
var ErrNoReconfiguration = errors.New(
	"no delivery reconfiguration pending",
)

const (
	generationAccepting generationState = "accepting"
	generationDraining  generationState = "draining"
	generationStopped   generationState = "stopped"
	generationFailed    generationState = "failed"
)

// providerGeneration is an immutable lookup snapshot for one configured
// provider set. Entries are values so a reconfiguration cannot invalidate a
// fallback while an older delivery is still finishing.
type providerGeneration struct {
	entries map[string]providerEntry
	order   []string
	state   generationState
}

type Manager struct {
	generation      *providerGeneration
	silences        []silenceMatcher
	templates       map[string]*template.Template
	clusterName     string
	providerDeps    transport.Dependencies
	started         bool
	stopped         bool
	reconfiguring   bool
	reconfigureDone chan struct{}
	reconfigureErr  error
	reconfigureWait bool
	generationStuck bool
	mu              sync.Mutex
	cfgMu           sync.RWMutex
	workerCount     int
	workerDone      chan struct{}
	workerCtx       context.Context
	cancelWorker    context.CancelFunc
	pending         []deliverJob
	dlqMu           sync.Mutex
	dlqRing         [dlqCap]DeadLetterEntry
	dlqHead         int
	dlqCount        int

	managerDone       chan struct{}
	managerDoneClosed bool
	reconfigureEvents chan struct{}
	lastProgress      atomic.Int64

	ctx context.Context
	now func() time.Time

	// pacer spreads deliveries per provider and holds the overflow digests.
	pacer sendPacer
}

func (a *Manager) nowTime() time.Time {
	return a.now()
}

// LastProgress implements the application lifecycle progress contract.
func (a *Manager) LastProgress() time.Time {
	value := a.lastProgress.Load()
	if value == 0 {
		return time.Time{}
	}
	return time.Unix(0, value)
}

func (a *Manager) touchProgress() {
	a.lastProgress.Store(a.nowTime().UnixNano())
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
) error {
	return a.initRuntime(runtime, factory)
}

func (a *Manager) initRuntime(
	runtime config.RuntimeConfig,
	factory ProviderFactory,
) error {
	a.mu.Lock()
	active := a.started && !a.stopped
	activeContext := a.ctx
	if a.generationStuck && a.workerCount > 0 {
		a.mu.Unlock()
		return fmt.Errorf("previous delivery generation is still stopping")
	}
	a.mu.Unlock()
	clusterName := runtime.Application().ClusterName
	entries, err := buildProviderEntries(runtime, factory, clusterName,
		a.providerDeps)
	if err != nil {
		return err
	}
	if active {
		if err := a.drainForReconfiguration(); err != nil {
			return err
		}
	}
	a.publishRuntime(entries, runtime, clusterName)
	if active {
		if err := a.Start(activeContext); err != nil {
			klog.ErrorS(err, "failed to restart delivery workers")
			a.finishReconfiguration(err)
			return err
		}
		a.finishReconfiguration(nil)
	}
	return nil
}

func buildProviderEntries(
	runtime config.RuntimeConfig,
	factory ProviderFactory,
	clusterName string,
	deps transport.Dependencies,
) ([]providerEntry, error) {
	providerContext := transport.ProviderContext{
		ClusterName: clusterName, Dependencies: deps,
	}
	providers := runtime.Delivery().Providers()
	entries := make([]providerEntry, 0, len(providers))
	for _, provider := range providers {
		entry, err := buildProviderEntry(provider, factory, providerContext)
		if err != nil {
			return nil, err
		}
		if entry != nil {
			entries = append(entries, *entry)
		}
	}
	logMissingFallbacks(entries)
	for _, providerName := range sanitizeFallbackCycles(entries) {
		klog.InfoS(
			"fallback cycle detected; disabling fallback",
			"provider", providerName,
		)
	}
	if err := validateProviderNames(entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func buildProviderEntry(
	provider config.ProviderRuntime,
	factory ProviderFactory,
	providerContext transport.ProviderContext,
) (*providerEntry, error) {
	name := strings.ToLower(provider.Name)
	var instance Provider
	if factory != nil {
		instance = factory(name, provider.Settings, providerContext)
	}
	if instance == nil {
		if config.IsKnownProvider(name) {
			return nil, fmt.Errorf(
				"provider %q could not be constructed; check its settings",
				provider.Name,
			)
		}
		return nil, fmt.Errorf("unknown alert provider %q", provider.Name)
	}
	if isNilProvider(instance) {
		return nil, nil
	}
	return &providerEntry{
		catalogName: name, provider: instance, routes: provider.Routes,
		retry:        retryConfigFromRuntime(provider.Retry),
		fallbackName: provider.FallbackName,
		templates:    compileTemplates(provider.Templates),
		maxBytes:     defaultMaxBytes(name),
		ch:           make(chan deliverJob, channelCap),
	}, nil
}

func logMissingFallbacks(entries []providerEntry) {
	for i := range entries {
		fallback := entries[i].fallbackName
		if fallback == "" || hasProvider(entries, fallback) {
			continue
		}
		klog.InfoS(
			"fallback provider not found, skipping",
			"provider", entries[i].provider.Name(),
			"fallback", fallback,
		)
	}
}

func hasProvider(entries []providerEntry, name string) bool {
	for _, entry := range entries {
		if strings.EqualFold(entry.lookupName(), name) {
			return true
		}
	}
	return false
}

func (a *Manager) drainForReconfiguration() error {
	a.mu.Lock()
	a.ensureLifecycleChannelsLocked()
	if a.reconfiguring {
		a.mu.Unlock()
		return fmt.Errorf("delivery reconfiguration already in progress")
	}
	a.reconfiguring = true
	a.reconfigureDone = make(chan struct{})
	a.reconfigureErr = nil
	a.reconfigureWait = true
	if a.generation != nil {
		a.generation.state = generationDraining
	}
	a.mu.Unlock()
	reconfigureCtx, cancel := context.WithTimeout(
		context.Background(), 10*time.Second,
	)
	defer cancel()
	if err := a.shutdownContext(reconfigureCtx); err != nil {
		klog.ErrorS(err,
			"delivery generation did not drain during reconfiguration")
		a.finishReconfiguration(err)
		return err
	}
	return nil
}

func (a *Manager) publishRuntime(
	items []providerEntry,
	runtime config.RuntimeConfig,
	clusterName string,
) {
	a.mu.Lock()
	a.generation = newProviderGeneration(items)
	a.pacer = sendPacer{}
	a.clusterName = clusterName
	a.started = false
	a.stopped = false
	a.ctx = nil
	a.mu.Unlock()
	a.cfgMu.Lock()
	deliveryRuntime := runtime.Delivery()
	a.silences = compileSilences(deliveryRuntime.Silences())
	a.templates = compileTemplates(deliveryRuntime.Templates())
	a.cfgMu.Unlock()
}

// IsReconfiguring reports whether workers are stopping as part of a runtime
// generation replacement rather than application shutdown.
func (a *Manager) IsReconfiguring() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.reconfiguring
}

func (a *Manager) finishReconfiguration(err error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.reconfiguring {
		return
	}
	a.reconfiguring = false
	a.reconfigureErr = err
	if a.reconfigureDone != nil {
		close(a.reconfigureDone)
	}
	if err != nil && a.workerCount == 0 {
		a.closeManagerDoneLocked()
	} else if a.reconfigureEvents != nil {
		select {
		case a.reconfigureEvents <- struct{}{}:
		default:
		}
	}
}

// WaitForReconfiguration waits until an in-progress generation replacement
// has either started the new workers or failed.
func (a *Manager) WaitForReconfiguration(ctx context.Context) error {
	a.mu.Lock()
	done := a.reconfigureDone
	pending := a.reconfigureWait
	a.mu.Unlock()
	if done == nil || !pending {
		return ErrNoReconfiguration
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-done:
		a.mu.Lock()
		err := a.reconfigureErr
		a.reconfigureWait = false
		a.reconfigureDone = nil
		a.mu.Unlock()
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// ReconfigurationEvents notifies the application when a generation replacement
// has completed. It is separate from Done, which represents manager shutdown.
func (a *Manager) ReconfigurationEvents() <-chan struct{} {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.ensureLifecycleChannelsLocked()
	return a.reconfigureEvents
}

func (a *Manager) ensureLifecycleChannelsLocked() {
	if a.managerDone == nil {
		a.managerDone = make(chan struct{})
	}
	if a.reconfigureEvents == nil {
		a.reconfigureEvents = make(chan struct{}, 1)
	}
}

func (a *Manager) closeManagerDoneLocked() {
	a.ensureLifecycleChannelsLocked()
	if a.managerDoneClosed {
		return
	}
	a.managerDoneClosed = true
	close(a.managerDone)
}

func validateProviderNames(entries []providerEntry) error {
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		name := entry.lookupName()
		if _, exists := seen[name]; exists {
			return fmt.Errorf("duplicate provider name %q", name)
		}
		seen[name] = struct{}{}
	}
	return nil
}

func newProviderGeneration(entries []providerEntry) *providerGeneration {
	generation := &providerGeneration{
		entries: make(map[string]providerEntry, len(entries)),
		order:   make([]string, 0, len(entries)),
		state:   generationAccepting,
	}
	for _, entry := range entries {
		name := entry.lookupName()
		if _, exists := generation.entries[name]; exists {
			klog.InfoS(
				"duplicate provider entry ignored",
				"provider", entry.provider.Name(),
			)
			continue
		}
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
