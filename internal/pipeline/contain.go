package pipeline

import (
	"fmt"

	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/metrics"
)

// contain runs one step of the decision path and reports whether it
// finished. A panic is contained at this boundary and counted: the step
// is skipped as a whole, the loop keeps its state and goes on, and the
// next observation or deadline runs the step again. Detector panics are
// contained one level lower, per entity; the root-cause solve and the
// incident manager are contained per solve, and a compose writer per
// decision, so one bad object cannot stop monitoring or crash-loop the
// process on replay.
func contain(stage, operation string, step func()) (ok bool) {
	defer func() {
		if r := recover(); r != nil {
			ok = false
			metrics.DefaultRegistry().IncPanic(stage)
			klog.ErrorS(nil, "pipeline: step panicked",
				"component", "pipeline", "operation", operation,
				"stage", stage, "panic", fmt.Sprint(r))
		}
	}()
	step()
	return true
}
