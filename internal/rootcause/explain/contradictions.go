package explain

import (
	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// A cause is attached only to a symptom it can produce. Rows match
// families of modes, and a family on the cause side can include a mode
// that rules out some effect modes of the same row: the symptom itself
// proves the cause did not happen to it. contradictions lists those
// pairs for every row at once, so a new row inherits them.
type contradiction struct {
	// cause are the cause modes, effect the effect modes they cannot
	// produce, ran the ones they cannot produce in a pod whose
	// containers have run, and why says so in plain words.
	cause, effect, ran []detection.Mode
	why                string
}

var contradictions = []contradiction{
	{
		cause: []detection.Mode{ModeMetricsUnserved,
			detection.ModeAPIServiceUnavailable},
		effect: []detection.Mode{detection.ModeScalingMaxedOut},
		why: "an autoscaler that computes a desired count above its " +
			"maximum has read its metrics",
	},
	{
		cause: []detection.Mode{detection.ModeNetworkIPExhausted,
			detection.ModeNetworkCNINotReady,
			detection.ModeNetworkConflict},
		effect: []detection.Mode{detection.ModeProbe,
			detection.ModeCrashLoop, detection.ModeRestarting,
			detection.ModeOOMKilled, detection.ModeExit},
		ran: []detection.Mode{detection.ModeNotReady},
		why: "a container that runs, or ran and restarts in its pod, " +
			"has its network: only a pod that cannot start lacks an " +
			"address",
	},
}

// contradicted are the effect modes a cause in mode cannot produce in
// st, whose containers have run (ran) or not. A node's own agent (a
// DaemonSet pod that serves its node, such as its network plugin) is
// exempt: it fails with its node, before the node shows it, and its
// crash is the node's problem (nodeDisowns).
func (st state) contradicted(mode detection.Mode) []detection.Mode {
	if st.agent {
		return nil
	}
	ran := st.ran
	var out []detection.Mode
	for _, c := range contradictions {
		if !anyModeMatches(c.cause, mode) {
			continue
		}
		out = append(out, c.effect...)
		if ran {
			out = append(out, c.ran...)
		}
	}
	return out
}

// hasRun reports a pod or container whose container has started: it
// runs now, or it restarted.
func (v *view) hasRun(id inventory.EntityID) bool {
	ids := []inventory.EntityID{id}
	if id.Kind == kube.KindPod {
		ids = v.s.Model.Related(id, inventory.PartOf, inventory.Incoming)
	}
	for _, c := range ids {
		e, ok := v.s.Model.Entity(c)
		if !ok || c.Kind != kube.KindContainer {
			continue
		}
		restarts, _ := attrNumber(e, kube.AttrRestarts)
		if attrText(e, kube.AttrState) == "running" || restarts > 0 {
			return true
		}
	}
	return false
}

// daemonPod reports a pod or container a DaemonSet runs.
func (v *view) daemonPod(id inventory.EntityID) bool {
	pod, ok := v.unitOf(id)
	return ok && rootcause.TopOwner(v.s.Model, pod).Kind ==
		kube.KindDaemonSet
}
