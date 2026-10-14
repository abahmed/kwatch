package detection

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection/upstream"
)

// externalEventReasons are event reasons from components outside
// kubernetes/kubernetes, which the generated corpus cannot hold.
var externalEventReasons = map[string]string{
	"FailedDeployModel": "AWS load balancer controller",
	"FailedBuildModel":  "AWS load balancer controller",
	"OOMKilling":        "node-problem-detector kernel monitor",
}

// A registry entry that names an event or state reason Kubernetes does
// not emit is a guess; it fails here until it is removed or renamed to
// the real reason.
func TestEventRegistryEntriesExistUpstream(t *testing.T) {
	have := map[string]bool{}
	for _, e := range upstream.Events {
		have[e.Reason] = true
	}
	for _, r := range upstream.HPAConditionReasons {
		have[r] = true
	}
	for _, list := range [][]ReasonInfo{
		eventOnlyReasons, upstreamReasons, admissionReasons,
	} {
		for _, info := range list {
			if info.Event == 0 && !info.ShownByState && info.Label == "" {
				continue
			}
			_, external := externalEventReasons[info.Reason]
			assert.True(t, have[info.Reason] != external,
				"%q: upstream=%v external=%v", info.Reason,
				have[info.Reason], external)
		}
	}
}
