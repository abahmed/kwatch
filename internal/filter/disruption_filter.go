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

	return StatusAlert
}

func (d DisruptionFilter) Execute(ctx *Context) bool {
	return d.Detect(ctx) == StatusSkip
}
