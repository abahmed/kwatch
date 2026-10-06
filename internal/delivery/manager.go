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
	// maxBytes is the provider's payload limit; 0 means no limit.
	maxBytes int
	// hourlyBudget is how many conversations the provider announces per
	// hour before the rest go to the overflow summary; 0 means no budget.
	hourlyBudget int
	ch           chan deliverJob
}

// ErrNoReconfiguration means the manager completion belonged to shutdown or
// an unexpected worker stop rather than a generation replacement.
var ErrNoReconfiguration = errors.New(
	"no delivery reconfiguration pending",
)

// Manager owns the provider generations and delivers notifications.
type Manager struct {
	generation   *providerGeneration
	templates    map[string]*template.Template
	clusterName  string
	providerDeps transport.Dependencies
	// state is the lifecycle state; see managerState.
	state       managerState
	reconfigure reconfigureOutcome
	mu          sync.Mutex
	cfgMu       sync.RWMutex
	workers     workerSet
	pending     []deliverJob
	onDelivered func()

	// outbox persists queued jobs; nil until the leader session attaches
	// the state store. restored holds the jobs it loaded until Start.
	outbox   atomic.Pointer[outbox]
	restored []deliverJob
	// handoff holds, per provider, the job a worker was delivering when
	// its context ended. Stop sends these first.
	handoff map[string][]deliverJob

	// done closes once the manager has stopped for good.
	done              doneSignal
	reconfigureEvents chan struct{}
	lastProgress      atomic.Int64

	ctx context.Context
	now func() time.Time

	// pacer spreads deliveries per provider and holds the overflow
	// summaries.
	pacer sendPacer
	// sleepFn replaces real waits in tests; nil waits on a timer.
	sleepFn func(context.Context, time.Duration) bool

	// backlog holds restored jobs per provider until its queue has room.
	backlog map[string][]deliverJob

	// busy counts workers in the middle of a job: pacing, waiting out a
	// provider, or sending. See LastProgress.
	busy atomic.Int32

	// told remembers which providers each conversation was routed to.
	told routeLedger
	// pagerLanded remembers which pagers accepted a message of each
	// conversation; pageObserver hears what became of the alerts.
	pagerLanded  routeLedger
	pageObserver atomic.Pointer[PageObserver]

	opensMu sync.Mutex
	// opens remembers, per provider and key, announcements that did not
	// reach the provider.
	opens   map[string]map[string]openEntry
	openSeq uint64

	healthMu        sync.Mutex
	providerReasons map[string]string
	healthEvents    chan struct{}
}

func (m *Manager) globalTemplates() map[string]*template.Template {
	m.cfgMu.RLock()
	defer m.cfgMu.RUnlock()
	return m.templates
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
func (m *Manager) InitRuntime(
	runtime config.RuntimeConfig,
	factory ProviderFactory,
) error {
	return m.initRuntime(runtime, factory)
}

func (m *Manager) initRuntime(
	runtime config.RuntimeConfig,
	factory ProviderFactory,
) error {
	active, activeContext, err := m.runtimeSnapshot()
	if err != nil {
		return err
	}
	clusterName := runtime.Application().ClusterName
	entries, err := buildProviderEntries(runtime, factory, clusterName,
		m.providerDeps)
	if err != nil {
		return err
	}
	if active {
		if err := m.drainForReconfiguration(); err != nil {
			return err
		}
	}
	// The new generation has new provider instances. Carry the thread ids
	// over, taken after the drain, or the next update of every open
	// incident would start a new thread.
	threads := m.SnapshotThreads()
	m.publishRuntime(entries, runtime, clusterName)
	m.RestoreThreads(threads)
	if active {
		if err := m.Start(activeContext); err != nil {
			klog.ErrorS(err, "failed to restart delivery workers")
			m.finishReconfiguration(err)
			return err
		}
		m.finishReconfiguration(nil)
	}
	return nil
}

// runtimeSnapshot reports whether workers are running, and with which
// context, so initRuntime knows whether it replaces a live generation.
func (m *Manager) runtimeSnapshot() (bool, context.Context, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.workers.stillStopping() {
		return false, nil, errGenerationStopping
	}
	return m.state.accepting(), m.ctx, nil
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
		entries = append(entries, *entry)
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
	if isNilProvider(instance) {
		// A constructor returns nil when a required setting is missing
		// or malformed. Skipping the provider would start kwatch with
		// alerts going nowhere, so startup and lint fail instead.
		if config.IsKnownProvider(name) {
			return nil, fmt.Errorf(
				"provider %q could not be constructed; check its settings",
				provider.Name,
			)
		}
		return nil, fmt.Errorf("unknown alert provider %q", provider.Name)
	}
	return &providerEntry{
		catalogName: name, provider: instance, routes: provider.Routes,
		retry:        retryConfigFromRuntime(provider.Retry),
		fallbackName: provider.FallbackName,
		templates:    compileTemplates(provider.Templates),
		maxBytes:     defaultMaxBytes(name),
		hourlyBudget: provider.HourlyBudget,
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

func (m *Manager) drainForReconfiguration() error {
	activeContext, err := m.beginReconfiguration()
	if err != nil {
		return err
	}
	if activeContext == nil {
		activeContext = context.Background()
	}
	reconfigureCtx, cancel := context.WithTimeout(
		context.WithoutCancel(activeContext), reconfigureDrainTimeout,
	)
	defer cancel()
	if err := m.shutdownContext(reconfigureCtx); err != nil {
		klog.ErrorS(err,
			"delivery generation did not drain during reconfiguration")
		m.finishReconfiguration(err)
		return err
	}
	return nil
}

// beginReconfiguration marks a generation replacement in progress and
// returns the context the current workers run with.
func (m *Manager) beginReconfiguration() (context.Context, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensureLifecycleChannelsLocked()
	if m.state.reconfiguring() {
		return nil, fmt.Errorf("delivery reconfiguration already in progress")
	}
	m.setStateLocked(m.state.reconfigureBegun())
	m.reconfigure = reconfigureOutcome{done: make(chan struct{})}
	if m.generation != nil {
		m.generation.state = generationDraining
	}
	return m.ctx, nil
}

func (m *Manager) publishRuntime(
	items []providerEntry,
	runtime config.RuntimeConfig,
	clusterName string,
) {
	m.mu.Lock()
	m.generation = newProviderGeneration(items)
	m.pacer.retain(pacerNames(items))
	m.clusterName = clusterName
	m.setStateLocked(m.state.afterPublish())
	m.ctx = nil
	names := append([]string(nil), m.generation.order...)
	m.mu.Unlock()
	m.configureProviderMetrics(names)
	m.cfgMu.Lock()
	deliveryRuntime := runtime.Delivery()
	m.templates = compileTemplates(deliveryRuntime.Templates())
	m.cfgMu.Unlock()
}

// pacerNames lists the keys the pacer may hold for these providers: the
// configured name and the provider's own name.
func pacerNames(items []providerEntry) map[string]struct{} {
	names := make(map[string]struct{}, 2*len(items))
	for _, item := range items {
		names[item.lookupName()] = struct{}{}
		names[item.provider.Name()] = struct{}{}
	}
	return names
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
