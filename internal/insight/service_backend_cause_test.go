package insight

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/constant"
	"github.com/abahmed/kwatch/internal/model"
)

func TestServiceBackendCauseRequiresObservedNodeFailure(t *testing.T) {
	inc := &model.Incident{
		Subject: model.Subject{Reason: constant.ReasonServiceNoEndpoints},
		Evidence: model.Evidence{Facts: model.Facts{
			BackendPods: 3, UnreadyBackendPods: 3,
			SharedFailingNode: "node-a",
			NodeFailureReason: "KubeletNotReady",
		}},
	}
	ins := &Insight{}
	assert.True(t, determineServiceBackendCause(inc, ins))
	assert.Contains(t, ins.Cause, "3 of 3 backend pods")
	assert.Contains(t, ins.Cause, "node-a")
	inc.Facts.SharedFailingNode = ""
	assert.False(t, determineServiceBackendCause(inc, &Insight{}))
	inc.Reason = constant.ReasonServiceBackendsDegraded
	inc.Facts.SharedFailingNode = "node-a"
	inc.Facts.UnreadyBackendPods = 1
	ins = &Insight{}
	assert.True(t, determineServiceBackendCause(inc, ins))
	assert.Contains(t, ins.Cause, "1 of 3 backend pods")
}

func TestServiceBackendCauseRanksFailingNode(t *testing.T) {
	inc := &model.Incident{
		Subject: model.Subject{
			Reason:   constant.ReasonServiceNoEndpoints,
			Resource: "service", Name: "api", Namespace: "prod",
		},
		Evidence: model.Evidence{Facts: model.Facts{
			SharedFailingNode: "node-a",
		}},
	}
	ins := &Insight{
		Cause:   "backend pods are on a failing node",
		Pattern: "service_node_failure", Confidence: 0.8,
	}
	(&Engine{}).finalizeAssessment(inc, ins)
	assert.Equal(t, model.ObjectRef{
		Kind: "node", Name: "node-a",
	}, ins.RootCause)
}
