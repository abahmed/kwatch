package workload

import (
	"fmt"
	"strings"
	"sync"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/client-go/tools/cache"

	"github.com/abahmed/kwatch/internal/config"
	"github.com/abahmed/kwatch/internal/model"
	"github.com/abahmed/kwatch/internal/monitor"
)

// LifecycleSink is the narrow incident boundary used by workload runtimes.
// It contains reconciliation, but no delivery or persistence mechanism.
type LifecycleSink interface {
	monitor.ObservationSink
	Reconcile(model.ObjectRef, []*model.Observation)
	ReconcileGone(model.ObjectRef)
}

// NodeIncidentCounter supplies the node attribution signal used by
// DaemonSets. It is separate from lifecycle reporting so workload runtimes
// do not need the complete incident engine.
type NodeIncidentCounter interface {
	CountActiveNodeIncidents() int
}

type runtimeSupport struct {
	runtime                 config.RuntimeConfig
	sink                    LifecycleSink
	now                     func() time.Time
	activeNodeIncidentCount func() int
	sourceMu                *sync.Mutex
	started                 bool
}

func newRuntimeSupportAt(
	runtime config.RuntimeConfig,
	sink LifecycleSink,
	now func() time.Time,
) runtimeSupport {
	return runtimeSupport{
		runtime:  runtime,
		sink:     sink,
		now:      now,
		sourceMu: &sync.Mutex{},
	}
}

// beginProcessing closes the source configuration window. Controller wiring
// happens before workers start, so a late setter cannot race with a lookup or
// silently change the behavior of an already-running runtime.
func (r *runtimeSupport) beginProcessing() {
	r.sourceMu.Lock()
	r.started = true
	r.sourceMu.Unlock()
}

func (r *runtimeSupport) configureSource(set func()) {
	r.sourceMu.Lock()
	defer r.sourceMu.Unlock()
	if !r.started {
		set()
	}
}

// sourceSnapshot reads a source under the same mutex used by
// configureSource. Source wiring is normally complete before workers start,
// but keeping the read synchronized makes the contract safe for embedded
// callers and race-enabled tests too.
func (r *runtimeSupport) sourceSnapshot[T any](read func() T) T {
	r.sourceMu.Lock()
	defer r.sourceMu.Unlock()
	return read()
}

// processKey contains the lifecycle shared by every workload queue runtime.
// The resource-specific callback retains detection and maintenance policy;
// this helper owns only key parsing, source availability, deletion, cache
// lookup, and NotFound handling.
func processKey[Source any, Object any](
	support *runtimeSupport,
	key, kind string,
	deleted bool,
	validate func(string, string) error,
	source func() (Source, bool),
	lookup func(Source, string, string) (Object, error),
	onGone func(model.ObjectRef),
	process func(model.ObjectRef, Object) error,
) error {
	support.beginProcessing()
	namespace, name, err := cache.SplitMetaNamespaceKey(key)
	if err != nil {
		return fmt.Errorf("invalid %s key %q: %w", kind, key, err)
	}
	if validate != nil {
		if err := validate(namespace, name); err != nil {
			return err
		}
	}
	subject := model.NewObjectRef(kind, namespace, name)
	sourceValue, available := source()
	if !available {
		return nil
	}
	if deleted {
		onGone(subject)
		return nil
	}
	value, err := lookup(sourceValue, namespace, name)
	if err != nil {
		if apierrors.IsNotFound(err) {
			onGone(subject)
			return nil
		}
		return fmt.Errorf(
			"failed to get %s %s/%s from cache: %w",
			kind, namespace, name, err,
		)
	}
	return process(subject, value)
}

func (r *runtimeSupport) observe(obs *model.Observation) {
	if obs == nil || r.sink == nil {
		return
	}
	prepareObservation(r.runtime, obs)
	r.sink.Process(obs)
}

func (r *runtimeSupport) reconcile(
	subject model.ObjectRef,
	obs []*model.Observation,
) {
	if r.sink == nil {
		return
	}
	for _, observation := range obs {
		prepareObservation(r.runtime, observation)
	}
	r.sink.Reconcile(subject, obs)
}

func (r *runtimeSupport) reconcileGone(subject model.ObjectRef) {
	if r.sink == nil {
		return
	}
	r.sink.ReconcileGone(subject)
}

func (r *runtimeSupport) maintenance(annotations map[string]string) bool {
	return inMaintenance(r.runtime.Maintenance(), annotations, r.now())
}

type firstSeen struct {
	mu     sync.Mutex
	values map[string]time.Time
}

func (s *firstSeen) mark(key string, now time.Time) time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.values == nil {
		s.values = make(map[string]time.Time)
	}
	if first, ok := s.values[key]; ok {
		return first
	}
	s.values[key] = now
	return now
}

func (s *firstSeen) clear(key string) {
	s.mu.Lock()
	delete(s.values, key)
	s.mu.Unlock()
}

func prepareObservation(runtime config.RuntimeConfig, obs *model.Observation) {
	if obs == nil {
		return
	}
	obs.IncludeEvents = runtime.IncludeEvents()
	obs.IncludeLogs = runtime.IncludeLogs()
}

// adaptiveSustained adds bounded grace only for a large workload with a small
// partial deficit. Single-replica and materially degraded workloads retain the
// configured sustain window.
func adaptiveSustained(
	baseMinutes int,
	enabled bool,
	desired, unavailable int32,
) time.Duration {
	if baseMinutes <= 0 {
		return 0
	}
	minutes := baseMinutes
	if enabled && desired >= 4 && unavailable > 0 && unavailable*4 <= desired {
		bonus := int(desired / 10)
		if bonus < 1 {
			bonus = 1
		}
		if bonus > 5 {
			bonus = 5
		}
		minutes += bonus
	}
	return time.Duration(minutes) * time.Minute
}

func observations(obs *model.Observation) []*model.Observation {
	if obs == nil {
		return nil
	}
	return []*model.Observation{obs}
}

func inMaintenance(
	maintenance config.MaintenanceConfig,
	annotations map[string]string,
	now time.Time,
) bool {
	if !maintenance.Enabled {
		return false
	}
	active := strings.EqualFold(strings.TrimSpace(
		annotations[maintenance.Annotation],
	), "true")
	if active || maintenance.UntilAnnotation == "" {
		return active
	}
	until, err := time.Parse(
		time.RFC3339,
		annotations[maintenance.UntilAnnotation],
	)
	return err == nil && now.Before(until)
}
