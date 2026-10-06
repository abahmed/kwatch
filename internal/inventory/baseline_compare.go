package inventory

// Comparison is a current value set against a workload's own normal
// range for a metric. It describes the past only: the range is what
// the workload did over the last week.
type Comparison struct {
	Metric Metric
	// Value is the current figure that was compared.
	Value float64
	// Typical is the usual figure and High the top of the normal range.
	Typical, High float64
	// Samples is how many samples the range rests on.
	Samples int
	// Known is false until the statistic has MinBaselineSamples
	// samples; a Comparison that is not Known judges nothing.
	Known bool
	// Usual is true when Value is inside the normal range.
	Usual bool
	// Unusual is true when Value is well above the normal range: over
	// unusualFactor times its top and by more than the metric's floor.
	Unusual bool
}

// Compare sets value against workload's normal range for metric.
func (b *Baselines) Compare(
	workload EntityID, metric Metric, value float64,
) Comparison {
	out := Comparison{Metric: metric, Value: value}
	stat, ok := b.Stat(workload, metric)
	if !ok || len(stat.Recent) < MinBaselineSamples {
		return out
	}
	out.Known = true
	out.Samples = len(stat.Recent)
	out.Typical, out.High = stat.Typical(), stat.High()
	out.Usual = value <= out.High
	out.Unusual = value > unusualFactor*out.High &&
		value > stat.Median()+floorOf(metric)
	return out
}

// TopOwner follows id's owners to the top-level controller: a pod's
// Deployment, a Job's CronJob. An object without an owner is its own
// workload. It stops after maxOwnerHops.
func TopOwner(model Reader, id EntityID) EntityID {
	for hop := 0; hop < maxOwnerHops; hop++ {
		owners := model.Related(id, OwnedBy, Outgoing)
		if len(owners) == 0 {
			break
		}
		id = owners[0]
	}
	return id
}

// maxOwnerHops bounds the owner chain followed to find a workload: pod,
// ReplicaSet, Deployment.
const maxOwnerHops = 3
