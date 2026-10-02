package kube

import (
	"context"
	"sort"
	"sync"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/inventory"
)

// DefaultOptionalSyncTimeout bounds the wait for optional kinds before the
// source reports them unavailable and lets the pipeline start without them.
const DefaultOptionalSyncTimeout = 2 * time.Minute

// syncPollInterval is how often WaitForSync rechecks informer state.
const syncPollInterval = 100 * time.Millisecond

// Source availability reason codes. They are a fixed vocabulary shared
// with health diagnostics; raw list errors stay in logs.
const (
	ReasonPermissionDenied = "permission_denied"
	ReasonAPIUnavailable   = "api_unavailable"
	ReasonSyncFailed       = "cache_sync_failed"
	ReasonSyncTimeout      = "cache_sync_timeout"
	ReasonSyncPending      = "cache_sync_pending"
	// ReasonDisabledByConfig marks a resource configuration turned off,
	// such as Secrets with watch.secrets: false.
	ReasonDisabledByConfig = "disabled_by_config"
)

// requiredResources are the kinds readiness waits for without a bound.
// Pods (with their containers) and nodes are what every failure story is
// about; without them kwatch cannot perform its monitoring function, so
// their absence must keep the leader not-ready. Every other kind, events
// included, only enriches or widens detection: a kind that cannot be
// listed (forbidden, not served) is reported unavailable and its
// detectors and rules skip it while the rest of kwatch runs.
var requiredResources = map[Resource]bool{
	{Name: "pods"}:  true,
	{Name: "nodes"}: true,
}

var eventsResource = Resource{Name: "events"}

var cancelledContext = func() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}()

func (r registration) required() bool {
	return requiredResources[r.resource]
}

// SourceStatus describes one watched resource that has not synced.
type SourceStatus struct {
	// Resource is the plural resource name, such as "secrets".
	Resource string
	Group    string
	Required bool
	// Reason is one of the Reason* codes.
	Reason string
}

// watchState tracks one informer's sync progress and last list failure.
type watchState struct {
	resource  Resource
	required  bool
	hasSynced cache.InformerSynced
	// lastVersion is the informer's last synced resource version.
	lastVersion func() string
	mu          sync.Mutex
	reason      string
	// lost is set when the API refused access after the initial sync
	// (a revoked Role). The cache then still holds the old objects, but
	// kwatch no longer sees changes, so the kind is not verifiable.
	// lostVersion is the resource version at that moment: a successful
	// relist moves it, which restores access.
	lost        bool
	lostVersion string
}

func (w *watchState) failure() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.reason
}

func (w *watchState) record(err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.reason = syncFailureReason(err)
	if w.reason == ReasonPermissionDenied && w.hasSynced() {
		w.lost, w.lostVersion = true, w.lastVersion()
	}
}

// accessLost reports whether the API refused the watch after it synced
// and no relist succeeded since. The resource version only moves once
// a list or watch got through again; on a fully idle cluster it may not
// move at all, and the kind stays unverifiable, which is the safe side.
func (w *watchState) accessLost() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.lost {
		return false
	}
	if w.lastVersion() != w.lostVersion {
		w.lost, w.reason = false, ""
		return false
	}
	return true
}

// track records an informer's sync state and classifies its list errors.
// It must run before the informer starts. Sync is the handler's, not the
// informer's: the informer syncs once its store holds the initial list,
// the handler only once it has submitted every object of that list, and
// only then does the model have them.
func (s *Source) track(
	resource Resource, required bool, informer cache.SharedIndexInformer,
	handle cache.ResourceEventHandlerRegistration,
) error {
	w := &watchState{
		resource: resource, required: required,
		hasSynced:   handle.HasSynced,
		lastVersion: informer.LastSyncResourceVersion,
	}
	s.watches = append(s.watches, w)
	return informer.SetWatchErrorHandlerWithContext(
		func(ctx context.Context, r *cache.Reflector, err error) {
			w.record(err)
			cache.DefaultWatchErrorHandler(ctx, r, err)
		})
}

func syncFailureReason(err error) string {
	switch {
	case apierrors.IsForbidden(err), apierrors.IsUnauthorized(err):
		return ReasonPermissionDenied
	case apierrors.IsNotFound(err), apierrors.IsMethodNotSupported(err),
		meta.IsNoMatchError(err):
		return ReasonAPIUnavailable
	default:
		return ReasonSyncFailed
	}
}

// settled reports whether an optional watch no longer holds up startup:
// it synced, or the API refused it in a way a retry will not fix soon.
func (w *watchState) settled() bool {
	if w.hasSynced() {
		return true
	}
	switch w.failure() {
	case ReasonPermissionDenied, ReasonAPIUnavailable:
		return true
	}
	return false
}

// WaitForSync blocks until every required kind synced and every optional
// kind synced, failed permanently or ran out of OptionalSyncTimeout. It
// reports false only when ctx ends first. Optional kinds that are still
// unsynced keep retrying in the background; Synced and Unavailable report
// their live state.
func (s *Source) WaitForSync(ctx context.Context) bool {
	select {
	case <-s.started:
	case <-ctx.Done():
		return false
	}
	deadline := time.NewTimer(s.cfg.OptionalSyncTimeout)
	defer deadline.Stop()
	poll := time.NewTicker(syncPollInterval)
	defer poll.Stop()
	for {
		if s.syncComplete() {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-deadline.C:
			s.syncExpired.Store(true)
		case <-poll.C:
		}
	}
}

func (s *Source) syncComplete() bool {
	expired := s.syncExpired.Load()
	for _, w := range s.watches {
		if w.required && !w.hasSynced() {
			return false
		}
		if !w.required && !expired && !w.settled() {
			return false
		}
	}
	return true
}

// accessLost reports whether the typed watch of kind lost its access
// after it synced (see watchState.accessLost).
func (s *Source) accessLost(kind inventory.Kind) bool {
	if kind == KindContainer {
		kind = KindPod
	}
	resource, ok := resourceOfKind()[kind]
	if !ok {
		return false
	}
	for _, w := range s.watches {
		if w.resource == resource {
			return w.accessLost()
		}
	}
	return false
}

// resourceOfKind maps each typed kind to the resource it is listed from.
var resourceOfKind = sync.OnceValue(func() map[inventory.Kind]Resource {
	out := map[inventory.Kind]Resource{}
	for _, r := range registrations() {
		out[r.schema.Kind()] = r.resource
	}
	return out
})

// Unavailable lists the watched resources that have not synced, or lost
// their access after they synced, sorted by group and resource, with a
// bounded reason code for each. A required resource listed here keeps
// the leader not ready.
func (s *Source) Unavailable() []SourceStatus {
	expired := s.syncExpired.Load()
	var out []SourceStatus
	for _, w := range s.watches {
		if w.hasSynced() {
			if w.accessLost() {
				out = append(out, SourceStatus{
					Resource: w.resource.Name, Group: w.resource.Group,
					Required: w.required, Reason: ReasonPermissionDenied,
				})
			}
			continue
		}
		reason := w.failure()
		switch {
		case reason == "" && expired:
			reason = ReasonSyncTimeout
		case reason == "":
			reason = ReasonSyncPending
		}
		out = append(out, SourceStatus{
			Resource: w.resource.Name, Group: w.resource.Group,
			Required: w.required, Reason: reason,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Group != out[j].Group {
			return out[i].Group < out[j].Group
		}
		return out[i].Resource < out[j].Resource
	})
	return out
}
