package detection

import (
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
)

// Context is what a detector may read. A detector whose condition depends
// on elapsed time calls RecheckAfter so the entity is evaluated again when
// the condition can change, instead of waiting for the next event.
type Context struct {
	Model inventory.Reader
	Now   time.Time

	recheck     *time.Duration
	synced      func(inventory.Kind) bool
	specialised bool
	onsets      *onsetScope
}

// Specialised reports whether a detector registered for the entity's own
// kind (not AnyKind) evaluates it. Fallback detectors leave conditions,
// phase and deletion of such kinds to that detector.
func (c Context) Specialised() bool { return c.specialised }

// Synced reports whether the kind is fully watched. Detectors must not
// conclude that an object is missing when its kind is not synced, for
// example because RBAC denies listing it.
func (c Context) Synced(kind inventory.Kind) bool {
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
	Findings []Finding
	// RecheckAfter is zero when no time-based condition is pending.
	RecheckAfter time.Duration
}

// Detector evaluates entities of the kinds it declares.
type Detector interface {
	// Name is a stable identifier used in diagnostics.
	Name() string
	// Kinds lists the entity kinds this detector evaluates.
	Kinds() []inventory.Kind
	// Detect returns the findings that currently hold for entity.
	Detect(ctx Context, entity inventory.Entity) []Finding
}

// Fallback marks a detector whose findings only fill gaps: it runs after
// every other detector of the entity, and the registry drops a fallback
// finding whose Mode a non-fallback finding of the same entity already
// reports, so an entity is never announced twice for one failure. A
// fallback condition finding (Mode prefixed ConditionMode) also yields to
// any non-fallback finding of the entity: the specialised detector has
// read the same status and named it better.
type Fallback interface {
	Detector
	// Fallback is a marker; it does nothing.
	Fallback()
}

// AnyKind registers a detector for every entity kind, such as one that
// reads Warning events attached to any object.
const AnyKind inventory.Kind = "*"

// Registry groups detectors by kind. It is built once at composition time;
// afterwards only its onset memory changes, as entities are evaluated.
type Registry struct {
	byKind map[inventory.Kind][]Detector
	synced func(inventory.Kind) bool
	onsets *onsets
}

// NewRegistry indexes detectors by kind. synced reports which kinds are
// fully watched; nil treats every kind as synced.
func NewRegistry(
	synced func(inventory.Kind) bool, detectors ...Detector,
) *Registry {
	r := &Registry{
		byKind: make(map[inventory.Kind][]Detector), synced: synced,
		onsets: newOnsets(),
	}
	for _, detector := range detectors {
		for _, kind := range detector.Kinds() {
			r.byKind[kind] = append(r.byKind[kind], detector)
		}
	}
	return r
}

// Evaluate runs every detector for the entity's kind. An absent entity is
// evaluated only by AnyKind detectors, which read notes (events) that can
// concern objects kwatch does not model.
func (r *Registry) Evaluate(
	model inventory.Reader, now time.Time, id inventory.EntityID,
) Evaluation {
	entity, present := model.Entity(id)
	if !present {
		entity = inventory.Entity{ID: id}
	}
	var recheck time.Duration
	scope := r.onsets.begin(id)
	defer r.onsets.finish(id, scope)
	ctx := Context{
		Model: model, Now: now, recheck: &recheck, synced: r.synced,
		specialised: present && len(r.byKind[id.Kind]) > 0,
		onsets:      scope,
	}
	groups := [][]Detector{r.byKind[AnyKind]}
	if present {
		groups = append(groups, r.byKind[id.Kind])
	}
	var out, fallback []Finding
	for _, group := range groups {
		for _, detector := range group {
			found := detect(ctx, detector, entity)
			if _, ok := detector.(Fallback); ok {
				fallback = append(fallback, found...)
			} else {
				out = append(out, found...)
			}
		}
	}
	out = append(out, withoutReportedModes(fallback, out)...)
	return Evaluation{Findings: collapseReasons(out), RecheckAfter: recheck}
}

// collapseReasons keeps one finding per reason of an entity, the most
// serious (the first on a tie), at the position of the first. The
// tracker keys findings by entity and reason, so a duplicate would
// silently overwrite or flap against its twin.
func collapseReasons(found []Finding) []Finding {
	index := make(map[string]int, len(found))
	out := found[:0]
	for _, f := range found {
		i, seen := index[f.Reason]
		if !seen {
			index[f.Reason] = len(out)
			out = append(out, f)
		} else if f.Severity > out[i].Severity {
			out[i] = f
		}
	}
	return out
}

// detect runs one detector and completes its findings.
func detect(
	ctx Context, detector Detector, entity inventory.Entity,
) []Finding {
	found := detector.Detect(ctx, entity)
	for i, s := range found {
		s = Classify(s)
		s.Entity = entity.ID
		if s.Since.IsZero() {
			s.Since = ctx.Now
		}
		found[i] = s
	}
	return found
}

// withoutReportedModes drops the fallback findings whose Mode one of the
// specialised findings already reports, and fallback condition findings
// when any specialised finding exists.
func withoutReportedModes(fallback, specialised []Finding) []Finding {
	if len(fallback) == 0 || len(specialised) == 0 {
		return fallback
	}
	reported := make(map[Mode]bool, len(specialised))
	for _, f := range specialised {
		reported[f.Mode] = true
	}
	kept := fallback[:0]
	for _, f := range fallback {
		if reported[f.Mode] || strings.HasPrefix(string(f.Mode), ConditionMode) {
			continue
		}
		kept = append(kept, f)
	}
	return kept
}
