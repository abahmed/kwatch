package pipeline

import (
	"k8s.io/klog/v2"

	"github.com/abahmed/kwatch/internal/detection"
)

func (e *Engine) progress() {
	if e.deps.Progress != nil {
		e.deps.Progress()
	}
}

func notify(ch chan struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}

// logTransitions writes one debug line per finding change. It is off by
// default; -v=4 shows why an incident gains or keeps a member.
func logTransitions(transitions []detection.Transition) {
	if !klog.V(4).Enabled() {
		return
	}
	for _, t := range transitions {
		klog.V(4).InfoS("pipeline: finding "+transitionName(t.Kind),
			"component", "pipeline", "entity", t.Finding.Entity.String(),
			"reason", t.Finding.Reason)
	}
}

func transitionName(kind detection.TransitionKind) string {
	switch kind {
	case detection.Raised:
		return "raised"
	case detection.Cleared:
		return "cleared"
	}
	return "changed"
}
