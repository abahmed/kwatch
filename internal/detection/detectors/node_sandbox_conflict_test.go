package detectors

import (
	"slices"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection/reasons"
)

const (
	hostPortMessage = "Failed to create pod sandbox: rpc error: code = " +
		"Unknown desc = failed to setup network for sandbox: cannot " +
		"open hostport 80 for pod web-1: listen tcp4 :80: bind: " +
		"address already in use"
	addressInUseMessage = "Failed to create pod sandbox: rpc error: " +
		"failed to setup network for sandbox: plugin type=\"bridge\" " +
		"failed (add): address already in use"
	routeExistsMessage = "Failed to create pod sandbox: failed to " +
		"setup network for sandbox: error adding host side routes " +
		"for interface: cali4f2a1b, error: route already exists for " +
		"an interface other than 'cali4f2a1b'"
)

// conflictOnNode reports whether the node gets a pod network conflict
// finding from pods failing with these messages.
func conflictOnNode(messages ...string) bool {
	model := newTestModel()
	sandboxPods(model, messages...)
	return slices.Contains(findingReasons(detectNode(model)),
		reasons.NodePodNetworkConflict)
}

// A hostPort taken on the node is the pod's own clash, never the node's
// pod network; an address in use takes two pods; a route of the node
// that clashes is shown by one.
func TestNodeNetworkConflictNeedsTheNode(t *testing.T) {
	assert.False(t, conflictOnNode(hostPortMessage, hostPortMessage),
		"hostPort")
	assert.False(t, conflictOnNode(addressInUseMessage),
		"one pod with an address in use")
	assert.True(t, conflictOnNode(addressInUseMessage, addressInUseMessage),
		"two pods with an address in use")
	assert.True(t, conflictOnNode(routeExistsMessage), "a route of the node")
}
