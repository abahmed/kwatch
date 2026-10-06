package kube

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/metadata"
	"k8s.io/client-go/tools/cache"
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/inventory"
)

// DefaultRediscoveryInterval is the safety timer for re-discovery. CRD
// and APIService changes trigger it sooner; the timer catches anything
// those watches miss and retries types the API refused.
const DefaultRediscoveryInterval = 30 * time.Minute

var (
	crdResource = schema.GroupVersionResource{
		Group: "apiextensions.k8s.io", Version: "v1",
		Resource: "customresourcedefinitions",
	}
	apiServiceResource = schema.GroupVersionResource{
		Group: "apiregistration.k8s.io", Version: "v1",
		Resource: "apiservices",
	}
)

// DynamicConfig configures the dynamic source. Discovery selects the
// resource types; Dynamic serves status watches and Metadata serves
// metadata watches. Without Metadata, metadata-mode types are skipped.
type DynamicConfig struct {
	Client    dynamic.Interface
	Metadata  metadata.Interface
	Discovery discovery.DiscoveryInterfaceWithContext
	Resync    time.Duration
	Now       func() time.Time
	Submit    Submit
	// Maintenance names the maintenance annotations; zero disables them.
	Maintenance MaintenanceAnnotations
	// Budget caps watched resource types; zero means
	// DefaultResourceBudget.
	Budget int
	// MaxObjectsPerKind and ObjectBudget cap observed objects; zero
	// means DefaultMaxObjectsPerKind and DefaultObjectBudget.
	MaxObjectsPerKind int
	ObjectBudget      int
	// Rediscovery is the safety timer; zero means
	// DefaultRediscoveryInterval.
	Rediscovery time.Duration
	// Reconciled, when set, receives the status after every discovery
	// pass. It runs on the source's goroutine and must not block.
	Reconciled func(DynamicStatus)
	// Noted, when set, lists the kinds that had Warning events since
	// the given time. Over the budget they are watched before kinds
	// nothing has complained about; nil ranks none.
	Noted func(since time.Time) map[inventory.Kind]bool
}

// notedWindow is how recent a Warning event must be to pull its kind
// into the watch budget.
const notedWindow = time.Hour

// DynamicStatus summarises the dynamic watch plan.
type DynamicStatus struct {
	// Watched counts running watches by mode.
	Watched map[WatchMode]int
	// Skipped counts discovered types left out by the budget.
	Skipped int
	// Unavailable counts types that could not be watched: refused by
	// the API, or metadata types without a metadata client.
	Unavailable int
	// Capped counts watched types with objects left out by an object
	// cap; they are also counted in Watched.
	Capped int
	// Reasons counts skipped, unavailable and capped types by reason
	// code.
	Reasons map[string]int
	// Kinds lists at most MaxDynamicStatusKinds of those types, sorted;
	// KindsTruncated is set when more were left out.
	Kinds          []DynamicKind
	KindsTruncated bool
	// Complete is false when the last discovery was partial.
	Complete bool
}

// DynamicSource discovers every served list-and-watch resource type the
// typed source does not cover and watches it in the mode its watch plan
// assigns (watchplan.go).
type DynamicSource struct {
	cfg     DynamicConfig
	budget  *objectBudget
	trigger chan struct{}
	mu      sync.Mutex
	running map[schema.GroupVersionResource]*dynamicWatch
	// refused holds types the API refused; they are retried on the
	// safety timer (retryDue), not on every triggered discovery.
	refused  map[schema.GroupVersionResource]*refusal
	retryDue bool
	// parked holds stopped watches of types the budget dropped. Their
	// entities stay in the model until the type is watched again.
	parked map[schema.GroupVersionResource]*dynamicWatch
	// left holds the discovered types the last pass did not watch:
	// over the budget, or with no client for their mode.
	left     map[schema.GroupVersionResource]kindIssue
	complete bool
	// loggedSkipped is the skipped-kind list last written to the log.
	loggedSkipped string
	wg            sync.WaitGroup
}

// kindIssue is why one resource type of an entity kind is not watched.
type kindIssue struct {
	kind   inventory.Kind
	reason string
}

// refusal is a type the API refused. A refusal that is not a removal
// keeps the stopped watch: its entities stay in the model, not
// verifiable, and the restarted watch takes them over.
type refusal struct {
	kindIssue
	kept *dynamicWatch
}

// dynamicWatch is one running informer and what is needed to retire it.
type dynamicWatch struct {
	kind inventory.Kind
	mode WatchMode
	// parent is the source context the watch was started under.
	parent     context.Context
	cancel     context.CancelFunc
	done       chan struct{}
	store      cache.Store
	hasSynced  cache.InformerSynced
	translator *Translator
	admission  *objectAdmission
	// handedOver is closed once a watch that replaced a previous one has
	// reported gone what disappeared in between; until then the kind is
	// not complete. It is nil for a watch without a predecessor.
	handedOver chan struct{}
}

// takingOver reports whether the watch still hands over from its
// predecessor.
func (w *dynamicWatch) takingOver() bool {
	if w.handedOver == nil {
		return false
	}
	select {
	case <-w.handedOver:
		return false
	default:
		return true
	}
}

// NewDynamicSource builds a dynamic source.
func NewDynamicSource(cfg DynamicConfig) *DynamicSource {
	if cfg.Budget <= 0 {
		cfg.Budget = DefaultResourceBudget
	}
	if cfg.MaxObjectsPerKind <= 0 {
		cfg.MaxObjectsPerKind = DefaultMaxObjectsPerKind
	}
	if cfg.ObjectBudget <= 0 {
		cfg.ObjectBudget = DefaultObjectBudget
	}
	if cfg.Rediscovery <= 0 {
		cfg.Rediscovery = DefaultRediscoveryInterval
	}
	return &DynamicSource{
		cfg:     cfg,
		budget:  &objectBudget{limit: int64(cfg.ObjectBudget)},
		trigger: make(chan struct{}, 1),
		running: make(map[schema.GroupVersionResource]*dynamicWatch),
		refused: make(map[schema.GroupVersionResource]*refusal),
		parked:  make(map[schema.GroupVersionResource]*dynamicWatch),
		left:    make(map[schema.GroupVersionResource]kindIssue),
	}
}

// Run discovers and watches resources until ctx ends, then waits for every
// informer to stop. It re-discovers when a CRD or APIService changes and
// on the safety timer.
func (d *DynamicSource) Run(ctx context.Context) {
	defer d.wg.Wait()
	ticker := time.NewTicker(d.cfg.Rediscovery)
	defer ticker.Stop()
	for {
		d.reconcile(ctx)
		select {
		case <-ctx.Done():
			d.stopAll()
			return
		case <-ticker.C:
			d.retryRefused()
		case <-d.trigger:
		}
	}
}

// Status returns the current watch plan summary.
func (d *DynamicSource) Status() DynamicStatus {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.statusLocked()
}

func (d *DynamicSource) statusLocked() DynamicStatus {
	out := DynamicStatus{
		Watched:  map[WatchMode]int{},
		Reasons:  map[string]int{},
		Complete: d.complete,
	}
	var kinds []DynamicKind
	note := func(gvr schema.GroupVersionResource, reason string) {
		out.Reasons[reason]++
		kinds = append(kinds, DynamicKind{
			Resource: resourceName(gvr), Reason: reason,
		})
	}
	for gvr, w := range d.running {
		out.Watched[w.mode]++
		if w.admission.capped() {
			out.Capped++
			note(gvr, ReasonObjectCap)
		}
	}
	for gvr, f := range d.refused {
		out.Unavailable++
		note(gvr, f.reason)
	}
	for gvr, issue := range d.left {
		if issue.reason == ReasonWatchBudget {
			out.Skipped++
		} else {
			out.Unavailable++
		}
		note(gvr, issue.reason)
	}
	out.Kinds, out.KindsTruncated = boundDynamicKinds(kinds)
	return out
}

// requestDiscovery schedules a discovery pass without blocking; pending
// requests coalesce.
func (d *DynamicSource) requestDiscovery() {
	select {
	case d.trigger <- struct{}{}:
	default:
	}
}

// retryRefused lets the next pass restart the types the API refused.
func (d *DynamicSource) retryRefused() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.retryDue = true
}

// reconcile starts informers for newly discovered resources and retires
// the ones no longer wanted. When discovery was incomplete nothing is
// retired: a transient API error must not look like every resource type
// being deleted.
func (d *DynamicSource) reconcile(ctx context.Context) {
	discovered, complete := discoverResources(ctx, d.cfg.Discovery)
	d.mu.Lock()
	desired, skipped := applyBudget(discovered, d.cfg.Budget,
		func(gvr schema.GroupVersionResource) bool {
			_, ok := d.running[gvr]
			return ok
		}, d.notedKinds())
	retry := d.retryDue
	d.retryDue = false
	d.complete = complete
	d.left = make(map[schema.GroupVersionResource]kindIssue)
	for _, r := range skipped {
		d.left[r.gvr] = kindIssue{KindFor(r.kind), ReasonWatchBudget}
	}
	for _, r := range desired {
		if _, running := d.running[r.gvr]; !running {
			d.watch(ctx, r, retry)
		}
	}
	retired := d.dropUnwanted(desired, skipped, complete)
	status := d.statusLocked()
	d.mu.Unlock()
	if kinds, changed := d.skippedChanged(skipped); changed {
		klog.InfoS("resource types over the watch budget are skipped",
			"component", "inventory", "operation", "discover",
			"discovered", len(discovered), "budget", d.cfg.Budget,
			"skipped", len(skipped), "kinds", kinds)
	}
	for _, w := range retired {
		d.retire(ctx, w)
	}
	if d.cfg.Reconciled != nil {
		d.cfg.Reconciled(status)
	}
}

// skippedChanged returns the skipped kinds and whether they differ from
// the list last logged, so the log names them at startup and again only
// when the list changes, not on every discovery pass. An empty list is
// never logged.
func (d *DynamicSource) skippedChanged(
	skipped []plannedResource,
) (string, bool) {
	kinds := skippedKinds(skipped)
	d.mu.Lock()
	defer d.mu.Unlock()
	changed := kinds != "" && kinds != d.loggedSkipped
	d.loggedSkipped = kinds
	return kinds, changed
}

// watch starts a wanted type that is not running, taking over a refused
// or parked watch's entities. A refused type waits for retry. The caller
// holds d.mu.
func (d *DynamicSource) watch(
	ctx context.Context, r plannedResource, retry bool,
) {
	if f, ok := d.refused[r.gvr]; ok {
		if retry && d.start(ctx, r, f.kept) {
			delete(d.refused, r.gvr)
		}
		return
	}
	prev := d.parked[r.gvr]
	if d.start(ctx, r, prev) {
		delete(d.parked, r.gvr)
		return
	}
	d.left[r.gvr] = kindIssue{KindFor(r.kind), ReasonAPIUnavailable}
}

// dropUnwanted stops the types the plan no longer wants and returns the
// watches whose entities are gone. A type the budget dropped is parked
// quietly: its objects still exist. A type no longer discovered is
// removed, but only after a complete discovery. The caller holds d.mu.
func (d *DynamicSource) dropUnwanted(
	desired, skipped []plannedResource, complete bool,
) []*dynamicWatch {
	wanted := gvrSet(desired)
	overBudget := gvrSet(skipped)
	var retired []*dynamicWatch
	for gvr, w := range d.running {
		switch {
		case wanted[gvr]:
		case overBudget[gvr]:
			w.cancel()
			delete(d.running, gvr)
			d.parked[gvr] = w
		case complete:
			w.cancel()
			delete(d.running, gvr)
			retired = append(retired, w)
		}
	}
	for gvr, w := range d.parked {
		if complete && !overBudget[gvr] && !wanted[gvr] {
			delete(d.parked, gvr)
			retired = append(retired, w)
		}
	}
	for gvr, f := range d.refused {
		if !complete || wanted[gvr] || overBudget[gvr] {
			continue
		}
		delete(d.refused, gvr)
		if f.kept != nil {
			retired = append(retired, f.kept)
		}
	}
	return retired
}

func gvrSet(
	resources []plannedResource,
) map[schema.GroupVersionResource]bool {
	out := make(map[schema.GroupVersionResource]bool, len(resources))
	for _, r := range resources {
		out[r.gvr] = true
	}
	return out
}

// stopAll cancels every informer on shutdown. Entities are not reported
// gone: the whole source is stopping, not the resource types.
func (d *DynamicSource) stopAll() {
	d.mu.Lock()
	defer d.mu.Unlock()
	for gvr, w := range d.running {
		w.cancel()
		w.admission.releaseAll()
		delete(d.running, gvr)
	}
	for gvr, w := range d.parked {
		w.admission.releaseAll()
		delete(d.parked, gvr)
	}
	for gvr, f := range d.refused {
		if f.kept != nil {
			f.kept.admission.releaseAll()
		}
		delete(d.refused, gvr)
	}
}

// skippedKinds names the kinds the budget left out, sorted, so an
// operator can raise the budget or recognise a gap without guessing. The
// list is bounded by what discovery returned beyond the budget.
func skippedKinds(skipped []plannedResource) string {
	names := make([]string, 0, len(skipped))
	for _, r := range skipped {
		names = append(names, r.kind)
	}
	sort.Strings(names)
	return strings.Join(names, ",")
}

// notedKinds asks the model which kinds had Warning events recently,
// once per reconcile; nil when the source has no model to ask.
func (d *DynamicSource) notedKinds() map[inventory.Kind]bool {
	if d.cfg.Noted == nil {
		return nil
	}
	return d.cfg.Noted(d.cfg.Now().Add(-notedWindow))
}
