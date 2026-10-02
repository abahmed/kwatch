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

// maxOwnerHops bounds the owner chain followed to find a workload: pod,
// ReplicaSet, Deployment.
const maxOwnerHops = 3

// baselineSampler turns observations into workload baseline samples. It
// runs on the decision loop and only touches memory. Each pod and Job
// counts once; container restart counts are compared with the last seen
// count.
type baselineSampler struct {
	model    *inventory.Model
	pending  map[inventory.EntityID]bool
	ready    map[inventory.EntityID]bool
	jobs     map[inventory.EntityID]bool
	restarts map[inventory.EntityID]float64
}

func newBaselineSampler(model *inventory.Model) *baselineSampler {
	return &baselineSampler{
		model:    model,
		pending:  make(map[inventory.EntityID]bool),
		ready:    make(map[inventory.EntityID]bool),
		jobs:     make(map[inventory.EntityID]bool),
		restarts: make(map[inventory.EntityID]float64),
	}
}

// observe samples one applied observation.
func (b *baselineSampler) observe(o inventory.Observation) {
	if o.Kind == inventory.Gone {
		b.forget(o.Entity)
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
	}
}

func (b *baselineSampler) forget(id inventory.EntityID) {
	delete(b.pending, id)
	delete(b.ready, id)
	delete(b.jobs, id)
	delete(b.restarts, id)
}

// samplePod records how long the pod waited to start and to be ready,
// both measured from its creation.
func (b *baselineSampler) samplePod(o inventory.Observation) {
	created := o.Attributes[kube.AttrCreated].AsTime()
	if created.IsZero() {
		return
	}
	workload := b.workload(o.Entity)
	if start := o.Attributes[kube.AttrStartTime].AsTime(); !start.IsZero() &&
		!b.pending[o.Entity] {
		b.pending[o.Entity] = true
		b.add(workload, inventory.MetricPendingSeconds, start.Sub(created), o.At)
	}
	ready, _ := o.Attributes[kube.AttrReady].AsBool()
	since := o.Attributes[kube.AttrReadySince].AsTime()
	if ready && !since.IsZero() && !b.ready[o.Entity] {
		b.ready[o.Entity] = true
		b.add(workload, inventory.MetricReadySeconds, since.Sub(created), o.At)
	}
}

// sampleContainer counts restarts since the container was last seen.
// The first sighting only sets the starting count.
func (b *baselineSampler) sampleContainer(o inventory.Observation) {
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

// workload follows id's owners to the top-level controller: a pod's
// Deployment, a Job's CronJob. An object without an owner is its own
// workload.
func (b *baselineSampler) workload(id inventory.EntityID) inventory.EntityID {
	for hop := 0; hop < maxOwnerHops; hop++ {
		owners := b.model.Related(id, inventory.OwnedBy, inventory.Outgoing)
		if len(owners) == 0 {
			break
		}
		id = owners[0]
	}
	return id
}
