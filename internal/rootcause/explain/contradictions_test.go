package explain

import (
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A cause is never attached to a symptom it cannot produce; the twin
// of each case is a symptom it can produce, still attached.
func TestContradictedEffectsAreNotExplained(t *testing.T) {
	metrics := inventory.CoreID(kube.KindAPIService, "",
		"v1beta1.metrics.k8s.io")
	hpa := inventory.CoreID(kube.KindHPA, "shop", "web")
	node := inventory.CoreID(kube.KindNode, "", "n1")
	pod := inventory.CoreID(kube.KindPod, "shop", "web-1")
	for _, tt := range []struct {
		name         string
		cause        state
		link         LinkType
		effect       state
		wantExplains bool
	}{
		{"metrics API down, autoscaler at its maximum",
			failingState(metrics, ModeMetricsUnserved), LinkServedBy,
			failingState(hpa, detection.ModeScalingMaxedOut), false},
		{"metrics API down, autoscaler without metrics",
			failingState(metrics, ModeMetricsUnserved), LinkServedBy,
			failingState(hpa, detection.ModeScalingNoMetrics), true},
		{"node out of addresses, running pod not ready",
			failingState(node, detection.ModeNetworkIPExhausted),
			LinkRunsOn, ranState(pod, detection.ModeNotReady), false},
		{"node out of addresses, pod not ready that never started",
			failingState(node, detection.ModeNetworkIPExhausted),
			LinkRunsOn, failingState(pod, detection.ModeNotReady), true},
		{"node out of addresses, container failing readiness",
			failingState(node, detection.ModeNetworkIPExhausted),
			LinkRunsOn, failingState(pod, detection.ModeProbeReadiness),
			false},
		{"node out of addresses, pod that cannot start",
			failingState(node, detection.ModeNetworkIPExhausted),
			LinkRunsOn, failingState(pod, detection.ModeCreating), true},
	} {
		_, got := bestRow(propagation, tt.cause, tt.link, tt.effect)
		if got != tt.wantExplains {
			t.Errorf("%s: explains = %v, want %v", tt.name, got,
				tt.wantExplains)
		}
	}
}

// failingState is id failing in modes.
func failingState(id inventory.EntityID, modes ...detection.Mode) state {
	st := state{id: id}
	for _, m := range modes {
		st.modes = append(st.modes, modeHealth{mode: m,
			health: detection.Failing})
	}
	return st
}

// ranState is failingState of an effect whose container has run.
func ranState(id inventory.EntityID, modes ...detection.Mode) state {
	st := failingState(id, modes...)
	st.ran = true
	return st
}

// Every contradiction names modes a finding can carry.
func TestContradictionModesAreKnown(t *testing.T) {
	for _, c := range contradictions {
		for _, m := range append(append([]detection.Mode{}, c.cause...),
			append(c.effect, c.ran...)...) {
			if !detection.KnownMode(m) && !isPseudo(m) {
				t.Errorf("unknown mode %q in contradiction %q", m, c.why)
			}
		}
	}
}
