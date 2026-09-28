package signal

import (
	"time"

	"github.com/abahmed/kwatch/internal/knowledge"
)

// Context is what a detector may read. A detector whose condition depends
// on elapsed time calls RecheckAfter so the entity is evaluated again when
// the condition can change, instead of waiting for the next event.
type Context struct {
	Model knowledge.Reader
	Now   time.Time

	recheck *time.Duration
	synced  func(knowledge.Kind) bool
}

// Synced reports whether the kind is fully watched. Detectors must not
// conclude that an object is missing when its kind is not synced, for
// example because RBAC denies listing it.
func (c Context) Synced(kind knowledge.Kind) bool {
	return c.synced == nil || c.synced(kind)
}

// RecheckAfter asks for another evaluation of this entity after delay. The
// earliest request wins.
func (c Context) RecheckAfter(delay time.Duration) {
	if c.recheck == nil || delay <= 0 {
		return
	}
	if *c.recheck == 0 || delay < *c.recheck {
		*c.recheck = delay
	}
}

// Evaluation is the result of evaluating one entity.
type Evaluation struct {
	Signals []Signal
	// RecheckAfter is zero when no time-based condition is pending.
	RecheckAfter time.Duration
}

// Detector evaluates entities of the kinds it declares.
type Detector interface {
	// Name is a stable identifier used in diagnostics.
	Name() string
	// Kinds lists the entity kinds this detector evaluates.
	Kinds() []knowledge.Kind
	// Detect returns the signals that currently hold for entity.
	Detect(ctx Context, entity knowledge.Entity) []Signal
}

// Registry groups detectors by kind. It is built once at composition time
// and read-only afterwards.
type Registry struct {
	byKind map[knowledge.Kind][]Detector
	synced func(knowledge.Kind) bool
}

// NewRegistry indexes detectors by kind. synced reports which kinds are
// fully watched; nil treats every kind as synced.
func NewRegistry(
	synced func(knowledge.Kind) bool, detectors ...Detector,
) *Registry {
	r := &Registry{
		byKind: make(map[knowledge.Kind][]Detector), synced: synced,
	}
	for _, detector := range detectors {
		for _, kind := range detector.Kinds() {
			r.byKind[kind] = append(r.byKind[kind], detector)
		}
	}
	return r
}

// Evaluate runs every detector for the entity's kind. An absent entity has
// no signals.
func (r *Registry) Evaluate(
	model knowledge.Reader, now time.Time, id knowledge.EntityID,
) Evaluation {
	entity, ok := model.Entity(id)
	if !ok {
		return Evaluation{}
	}
	var recheck time.Duration
	ctx := Context{
		Model: model, Now: now, recheck: &recheck, synced: r.synced,
	}
	var out []Signal
	for _, detector := range r.byKind[id.Kind] {
		for _, s := range detector.Detect(ctx, entity) {
			s.Entity = id
			if s.Since.IsZero() {
				s.Since = now
			}
			out = append(out, s)
		}
	}
	return Evaluation{Signals: out, RecheckAfter: recheck}
}
