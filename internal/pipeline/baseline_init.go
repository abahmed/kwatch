package pipeline

import (
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// sampleInit records how long an init container's successful run took,
// once per container, so a later run that takes far longer is known to
// be unusual for this workload.
func (b *baselineSampler) sampleInit(o inventory.Observation) {
	if b.inits[o.Entity] {
		return
	}
	seconds, ok := o.Attributes[kube.AttrInitSeconds].AsNumber()
	if !ok {
		return
	}
	pods := b.model.Related(o.Entity, inventory.PartOf, inventory.Outgoing)
	if len(pods) == 0 {
		return
	}
	b.inits[o.Entity] = true
	b.add(b.workload(pods[0]), inventory.MetricInitSeconds,
		time.Duration(seconds*float64(time.Second)), o.At)
}
