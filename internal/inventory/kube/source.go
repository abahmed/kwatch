package kube

import (
	"context"
	"errors"
	"sync/atomic"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/kubeclient"
)

// Submit delivers observations to the pipeline.
type Submit func(ctx context.Context, observations ...inventory.Observation)

// SourceConfig configures the Kubernetes source.
type SourceConfig struct {
	Client kubernetes.Interface
	Resync time.Duration
	Now    func() time.Time
	Submit Submit
	// Maintenance names the maintenance annotations; zero disables them.
	Maintenance MaintenanceAnnotations
	// OptionalSyncTimeout bounds how long WaitForSync waits for optional
	// kinds; zero means DefaultOptionalSyncTimeout.
	OptionalSyncTimeout time.Duration
	// Dynamic, when set, answers Synced and Verifiable for the kinds the
	// typed informers do not watch (normally the DynamicSource).
	Dynamic DynamicKinds
	// DisableSecrets leaves Secrets unwatched (watch.secrets: false).
	// Secret is then never synced or verifiable, so checks that need
	// Secrets report that they cannot verify instead of guessing.
	DisableSecrets bool
	// DigestKey keys the Secret and ConfigMap value digests. Pass the same
	// stored key after a restart so digests stay comparable and downtime
	// changes are found; empty uses a random per-process key.
	DigestKey []byte
}

// DynamicKinds reports the state of dynamically watched kinds.
type DynamicKinds interface {
	KindState(kind inventory.Kind) (KindState, bool)
}

// mode is the watch mode of a typed registration.
func (r registration) mode() WatchMode {
	if hashedResources[r.resource] {
		return WatchHashed
	}
	return WatchFull
}

// newTransform keeps informer caches small and free of secret material:
// Secret and ConfigMap values are replaced by digests keyed with d.
func newTransform(d Digester) cache.TransformFunc {
	return func(obj any) (any, error) {
		obj, err := TrimToLatestManager(obj)
		if err != nil {
			return obj, err
		}
		dropLastAppliedTyped(obj)
		switch obj.(type) {
		case *corev1.Secret:
			return d.HashSecretData(obj)
		case *corev1.ConfigMap:
			return d.HashConfigMapData(obj)
		}
		return obj, nil
	}
}

// dropLastAppliedTyped removes the kubectl last-applied copy of an object
// from its annotations. It repeats the whole object, so keeping it in the
// cache can double what the cache holds; nothing reads it.
func dropLastAppliedTyped(obj any) {
	meta, ok := obj.(metav1.Object)
	if !ok {
		return
	}
	annotations := meta.GetAnnotations()
	if _, found := annotations[lastAppliedAnnotation]; found {
		delete(annotations, lastAppliedAnnotation)
		meta.SetAnnotations(annotations)
	}
}

// Source streams observations from Kubernetes informers.
type Source struct {
	cfg     SourceConfig
	factory informers.SharedInformerFactory
	synced  map[inventory.Kind]cache.InformerSynced
	watches []*watchState
	// disabled are the resources configuration turned off.
	disabled []registration
	started  chan struct{}
	// lifecycle is the Run context. Handlers submit with it, so a handler
	// blocked on a full pipeline returns when the source stops.
	lifecycle atomic.Pointer[context.Context]
	// syncExpired is set once the optional sync wait has elapsed.
	syncExpired atomic.Bool
}

// NewSource registers an informer and handler for every schema.
func NewSource(cfg SourceConfig) (*Source, error) {
	if cfg.Client == nil || cfg.Submit == nil || cfg.Now == nil {
		return nil, errors.New("kube source: incomplete configuration")
	}
	if cfg.OptionalSyncTimeout <= 0 {
		cfg.OptionalSyncTimeout = DefaultOptionalSyncTimeout
	}
	factory := informers.NewSharedInformerFactoryWithOptions(
		cfg.Client, cfg.Resync,
		informers.WithTransform(newTransform(NewDigester(cfg.DigestKey))),
	)
	s := &Source{
		cfg: cfg, factory: factory,
		synced:  make(map[inventory.Kind]cache.InformerSynced),
		started: make(chan struct{}),
	}
	for _, r := range registrations() {
		if cfg.DisableSecrets && r.resource == SecretsResource {
			s.disabled = append(s.disabled, r)
			continue
		}
		informer := r.informer(factory)
		// A panicking translator must not stop the informer from
		// delivering later updates; SafeEventHandler counts and logs it.
		handler := kubeclient.SafeEventHandler("inventory", r.resource.Name,
			translatorHandler(s.context,
				NewTranslator(withGenericAttributes(r.schema, r.mode())).
					WithMaintenance(cfg.Maintenance),
				cfg.Submit, cfg.Now))
		handle, err := informer.AddEventHandler(handler)
		if err != nil {
			return nil, err
		}
		err = s.track(r.resource, r.required(), informer, handle)
		if err != nil {
			return nil, err
		}
		s.synced[r.schema.Kind()] = handle.HasSynced
	}
	events := warningEventInformer(factory)
	handle, err := events.AddEventHandler(kubeclient.SafeEventHandler(
		"inventory", eventsResource.Name, s.eventHandler()))
	if err != nil {
		return nil, err
	}
	if err := s.track(eventsResource, false, events, handle); err != nil {
		return nil, err
	}
	return s, nil
}

// context returns the Run context, or a cancelled context before Run.
func (s *Source) context() context.Context {
	if ctx := s.lifecycle.Load(); ctx != nil {
		return *ctx
	}
	return cancelledContext
}

// eventHandler records Warning events as notes. Deleting an Event object
// (expiry) keeps the note: the evidence outlives the event.
func (s *Source) eventHandler() cache.ResourceEventHandler {
	note := func(obj any) {
		if observation, ok := EventNote(obj, s.cfg.Now()); ok {
			s.cfg.Submit(s.context(), observation)
		}
	}
	return cache.ResourceEventHandlerFuncs{
		AddFunc:    note,
		UpdateFunc: func(_, new any) { note(new) },
	}
}

// Run starts the informers and blocks until ctx ends and every informer
// has stopped.
func (s *Source) Run(ctx context.Context) {
	s.lifecycle.Store(&ctx)
	s.factory.Start(ctx.Done())
	close(s.started)
	<-ctx.Done()
	s.factory.Shutdown()
}

// Synced reports whether a kind has completed its initial list. Kinds
// without an informer are never synced. Detectors and rules must treat an
// unsynced kind as unknown: skip it, never create or resolve an incident
// from its absence.
func (s *Source) Synced(kind inventory.Kind) bool {
	if s.isDisabled(kind) {
		return false
	}
	if fn, ok := s.typedSynced(kind); ok {
		return fn()
	}
	state, ok := s.dynamicState(kind)
	return ok && state.Synced
}

func (s *Source) typedSynced(kind inventory.Kind) (cache.InformerSynced, bool) {
	fn, ok := s.synced[kind]
	if !ok && kind == KindContainer {
		// Containers are described with their pods.
		fn, ok = s.synced[KindPod]
	}
	return fn, ok
}

func (s *Source) dynamicState(kind inventory.Kind) (KindState, bool) {
	if s.cfg.Dynamic == nil {
		return KindState{}, false
	}
	return s.cfg.Dynamic.KindState(kind)
}

// Verifiable reports whether the source can currently observe kind.
// Kinds no source watches (virtual entities such as registries or
// zones) are verifiable: their state is derived, not listed. A watched
// kind that has not synced, for example because a permission is
// missing, is not; nor is a dynamic kind over a watch or object cap.
// A typed kind whose access was refused after it synced is not
// verifiable either, until a relist succeeds: its cache is stale.
func (s *Source) Verifiable(kind inventory.Kind) bool {
	if s.isDisabled(kind) {
		return false
	}
	if fn, ok := s.typedSynced(kind); ok {
		return fn() && !s.accessLost(kind)
	}
	state, ok := s.dynamicState(kind)
	return !ok || state.Synced
}

// isDisabled reports whether configuration turned kind's watch off.
func (s *Source) isDisabled(kind inventory.Kind) bool {
	for _, r := range s.disabled {
		if r.schema.Kind() == kind {
			return true
		}
	}
	return false
}

// Disabled lists the resources configuration turned off, with the reason
// ReasonDisabledByConfig. They are not failures: health reports them as
// not watched without degrading.
func (s *Source) Disabled() []SourceStatus {
	out := make([]SourceStatus, 0, len(s.disabled))
	for _, r := range s.disabled {
		out = append(out, SourceStatus{
			Resource: r.resource.Name, Group: r.resource.Group,
			Reason: ReasonDisabledByConfig,
		})
	}
	return out
}

// translatorHandler submits the observations a translator produces for informer
// notifications, with the context lifecycle returns at submit time.
func translatorHandler(
	lifecycle func() context.Context,
	t *Translator, submit Submit, now func() time.Time,
) cache.ResourceEventHandler {
	send := func(observations []inventory.Observation) {
		if len(observations) > 0 {
			submit(lifecycle(), observations...)
		}
	}
	return cache.ResourceEventHandlerDetailedFuncs{
		AddFunc: func(obj any, initialList bool) {
			send(t.Added(obj, initialList, now()))
		},
		UpdateFunc: func(old, new any) {
			send(t.Updated(old, new, now()))
		},
		DeleteFunc: func(obj any) {
			send(t.Deleted(obj, now()))
		},
	}
}
