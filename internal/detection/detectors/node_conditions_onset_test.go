package detectors

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A condition with no transition time starts its sustain wait when it
// turns True, not while it was still fine: a condition that has been
// False for a while is not reported the moment it turns True.
func TestNodeConditionWaitStartsWhenItTurnsTrue(t *testing.T) {
	m := inventory.NewModel(inventory.Options{})
	node := inventory.EntityID{Kind: kube.KindNode, Name: "worker-4"}
	set := func(at time.Time, status string) {
		put(m, node, at, map[string]inventory.Value{
			kube.ConditionKey("Ready"):          inventory.Text("True"),
			kube.ConditionKey("KernelDeadlock"): inventory.Text(status),
		})
	}
	r := detection.NewRegistry(nil, NewNode(0))
	set(t0, "False")
	assert.Empty(t, r.Evaluate(m, t0, node).Findings)

	turned := t0.Add(10 * time.Minute)
	set(turned, "True")
	assert.Empty(t, r.Evaluate(m, turned, node).Findings,
		"reported without its sustain wait")
	late := r.Evaluate(m, turned.Add(DefaultNodePressure), node)
	require.Len(t, late.Findings, 1)
}
