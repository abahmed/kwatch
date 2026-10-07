package pipeline

import (
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// maxBaselineDelay drops samples longer than this: a pod that became
// ready hours after creation was restarted or stuck, and one such value
// would distort the ready-time baseline.
const maxBaselineDelay = time.Hour

// baselineSampler turns observations into workload baseline samples. It
// runs on the decision loop and only touches memory. Each pod and Job
// counts once; container restart counts are compared with the last seen
// count.
type baselineSampler struct {
	model    *inventory.Model
	pending  map[inventory.EntityID]bool
	ready    map[inventory.EntityID]bool
	jobs     map[inventory.EntityID]bool
	inits    map[inventory.EntityID]bool
	restarts map[inventory.EntityID]float64
	// warnings is the event count already added, per entity and reason,
	// so a repeated event adds only its new occurrences.
	warnings map[inventory.EntityID]map[string]int
	// sweepAt is the warnings size that triggers the next sweep.
	sweepAt int
}

func newBaselineSampler(model *inventory.Model) *baselineSampler {
	return &baselineSampler{
		model:    model,
		pending:  make(map[inventory.EntityID]bool),
		ready:    make(map[inventory.EntityID]bool),
		jobs:     make(map[inventory.EntityID]bool),
		inits:    make(map[inventory.EntityID]bool),
		restarts: make(map[inventory.EntityID]float64),
		warnings: make(map[inventory.EntityID]map[string]int),
	}
}

// observe samples one applied observation.
func (b *baselineSampler) observe(o inventory.Observation) {
	if o.Kind == inventory.Gone {
		b.forget(o.Entity)
		return
	}
	if o.Kind == inventory.Noted {
		b.sampleWarning(o)
		return
	}
	if o.Kind != inventory.Observed {
		return
	}
	switch o.Entity.Kind {
	case kube.KindPod:
		b.samplePod(o)
	case kube.KindContainer:
		b.sampleContainer(o)
	case kube.KindJob:
		b.sampleJob(o)
	case kube.KindDeployment, kube.KindStatefulSet, kube.KindDaemonSet:
		b.sampleTemplate(o)
	}
}

// sampleTemplate tells the baseline which pod template the workload
// runs, so memory learned for old code is dropped when new code rolls
// out.
func (b *baselineSampler) sampleTemplate(o inventory.Observation) {
	template := o.Attributes[kube.AttrTemplateHash].AsText()
	if template == "" {
		template = o.Attributes[kube.AttrRevision].AsText()
	}
	b.model.Baselines().Touch(o.Entity, o.At)
	b.model.Baselines().Rebase(o.Entity, template, o.At)
}

// sampleWarning counts Warning events per hour for the workload the
// event is about. Each report adds only the occurrences it adds to the
// count already seen.
func (b *baselineSampler) sampleWarning(o inventory.Observation) {
	note := o.Note
	if !note.Warning || note.Reason == "" {
		return
	}
	seen := b.warnings[o.Entity]
	if seen == nil {
		seen = make(map[string]int)
		b.warnings[o.Entity] = seen
	}
	total := max(note.Count, 1)
	added := max(total-seen[note.Reason], 1)
	seen[note.Reason] = total
	b.model.Baselines().AddWarning(
		b.workload(o.Entity), note.Reason, added, o.At)
	b.sweepWarnings()
}

// warningSweepFloor is how many entities' warning counters the sampler
// holds before it looks for counters of entities that are gone.
const warningSweepFloor = 4096

// sweepWarnings drops the counters of entities that are not in the
// model. A late Warning event about a deleted pod (events outlive pods
// by an hour) creates a counter after the pod's Gone was seen, so
// nothing else would ever remove it. The sweep runs when the map has
// doubled since the last one, so its cost is amortized.
func (b *baselineSampler) sweepWarnings() {
	if len(b.warnings) < max(b.sweepAt, warningSweepFloor) {
		return
	}
	for id := range b.warnings {
		if !b.model.Exists(id) {
			delete(b.warnings, id)
		}
	}
	b.sweepAt = 2 * len(b.warnings)
}

func (b *baselineSampler) forget(id inventory.EntityID) {
	delete(b.pending, id)
	delete(b.ready, id)
	delete(b.jobs, id)
	delete(b.inits, id)
	delete(b.restarts, id)
	delete(b.warnings, id)
}

// samplePod records how long the pod waited to start and to be ready,
// both measured from its creation.
func (b *baselineSampler) samplePod(o inventory.Observation) {
	created := o.Attributes[kube.AttrCreated].AsTime()
	if created.IsZero() {
		return
	}
	workload := b.workload(o.Entity)
	if workload != o.Entity {
		// A pod seen before its owner is known has no workload yet.
		b.model.Baselines().Touch(workload, o.At)
	}
	if start := o.Attributes[kube.AttrStartTime].AsTime(); !start.IsZero() &&
		!b.pending[o.Entity] {
		b.pending[o.Entity] = true
		b.add(workload, inventory.MetricPendingSeconds,
			start.Sub(created), o.At)
	}
	ready, _ := o.Attributes[kube.AttrReady].AsBool()
	since := o.Attributes[kube.AttrReadySince].AsTime()
	if ready && !since.IsZero() && !b.ready[o.Entity] {
		b.ready[o.Entity] = true
		b.add(workload, inventory.MetricReadySeconds, since.Sub(created), o.At)
		b.sampleStart(workload, o, since)
	}
}

// sampleStart records how long the pod's containers took from starting
// to being ready. A pod whose containers restarted counts from the last
// start, so the value is what one start needs, not what the kills cost.
func (b *baselineSampler) sampleStart(
	workload inventory.EntityID, o inventory.Observation, ready time.Time,
) {
	started := o.Attributes[kube.AttrContainersStarted].AsTime()
	if started.IsZero() {
		started = o.Attributes[kube.AttrStartTime].AsTime()
	}
	if started.IsZero() || ready.Before(started) {
		return
	}
	b.add(workload, inventory.MetricStartSeconds, ready.Sub(started), o.At)
}

// sampleContainer counts restarts since the container was last seen.
// The first sighting only sets the starting count.
func (b *baselineSampler) sampleContainer(o inventory.Observation) {
	b.sampleMemory(o)
	b.sampleInit(o)
	count, ok := o.Attributes[kube.AttrRestarts].AsNumber()
	if !ok {
		return
	}
	previous, seen := b.restarts[o.Entity]
	b.restarts[o.Entity] = count
	if !seen || count <= previous {
		return
	}
	pods := b.model.Related(o.Entity, inventory.PartOf, inventory.Outgoing)
	if len(pods) == 0 {
		return
	}
	b.model.Baselines().AddRestarts(
		b.workload(pods[0]), int(count-previous), o.At)
}

// sampleMemory records the container's memory: the resident set when
// the kubelet reports it, else the working set.
func (b *baselineSampler) sampleMemory(o inventory.Observation) {
	bytes, ok := o.Attributes[kube.AttrMemoryRSS].AsNumber()
	if !ok {
		bytes, ok = o.Attributes[kube.AttrMemoryWorking].AsNumber()
	}
	if !ok || bytes <= 0 {
		return
	}
	pods := b.model.Related(o.Entity, inventory.PartOf, inventory.Outgoing)
	if len(pods) == 0 {
		return
	}
	b.model.Baselines().AddMemory(b.workload(pods[0]), bytes, o.At)
}

// completeCondition is the Job condition set when it succeeded.
const completeCondition = kube.AttrConditionPrefix + "Complete"

// sampleJob records how long a completed Job ran.
func (b *baselineSampler) sampleJob(o inventory.Observation) {
	if b.jobs[o.Entity] || o.Attributes[completeCondition].AsText() != "True" {
		return
	}
	start := o.Attributes[kube.AttrStartTime].AsTime()
	done := o.Attributes[completeCondition+kube.AttrConditionSince].AsTime()
	if start.IsZero() || done.IsZero() {
		return
	}
	b.jobs[o.Entity] = true
	b.add(b.workload(o.Entity), inventory.MetricJobSeconds, done.Sub(start),
		o.At)
}

func (b *baselineSampler) add(
	workload inventory.EntityID, metric inventory.Metric,
	value time.Duration, at time.Time,
) {
	if value < 0 || value > maxBaselineDelay {
		return
	}
	b.model.Baselines().Add(workload, metric, value.Seconds(), at)
}

// workload follows id's owners to the top-level controller.
func (b *baselineSampler) workload(id inventory.EntityID) inventory.EntityID {
	return inventory.TopOwner(b.model, id)
}
