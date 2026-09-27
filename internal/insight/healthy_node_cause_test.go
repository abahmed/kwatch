package insight

import (
	"testing"

	"github.com/stretchr/testify/assert"

	context "github.com/abahmed/kwatch/internal/graphcontext"
	"github.com/abahmed/kwatch/internal/model"
)

func TestAnalyzeDoesNotBlameHealthyNodeForPodCrash(t *testing.T) {
	graph := context.NewResourceGraph()
	graph.AddEdge("pod", "ns1", "p1", "node", "", "n1", "scheduled_on")
	graph.AddEdge("pod", "ns1", "p1", "deployment", "ns1", "api", "owned_by")
	e := withActiveNode(newTestEngine(graph, newTestChangeTracker(10)))
	ins := e.Analyze(&model.Incident{Subject: model.Subject{
		Resource: "pod", Namespace: "ns1", Name: "p1",
		NodeName: "n1", OwnerKind: "Deployment",
		Reason: "CrashLoopBackOff",
	}})
	assert.NotContains(t, ins.Cause, "node n1")
	assert.NotEqual(t, "node_failure", ins.Pattern)
	assert.NotEqual(t, CauseConfirmed, ins.CauseState)
}
