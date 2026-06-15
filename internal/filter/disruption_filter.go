package filter

// disruptionReasons is the set of DisruptionTarget condition reasons that
// indicate a controlled termination (not a crash).
var disruptionReasons = map[string]bool{
	"TerminationByKubelet":        true,
	"PreemptionByScheduler":       true,
	"EvictionByEvictionAPI":       true,
	"DeletionByTaintManager":      true,
	"DeletionByPodGC":             true,
	"ScaleDown":                   true,
	"DeletionByClusterAutoscaler": true,
}

type DisruptionFilter struct{}

func (d DisruptionFilter) Detect(ctx *Context) Status {
	if ctx.Pod == nil {
		return StatusAlert
	}

	if ctx.Pod.DeletionTimestamp != nil {
		return StatusSkip
	}

	for _, c := range ctx.Pod.Status.Conditions {
		if c.Type == "DisruptionTarget" && disruptionReasons[c.Reason] {
			return StatusSkip
		}
	}

	// classic kubelet node-pressure eviction: phase=Failed, reason=Evicted,
	// no DeletionTimestamp / DisruptionTarget condition. The root cause is a
	// node condition (NodeMonitor reports it); the per-pod symptom is noise.
	if string(ctx.Pod.Status.Phase) == "Failed" && ctx.Pod.Status.Reason == "Evicted" {
		return StatusSkip
	}

	return StatusAlert
}

func (d DisruptionFilter) Execute(ctx *Context) bool {
	return d.Detect(ctx) == StatusSkip
}
