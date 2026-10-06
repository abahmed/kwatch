package scenarios

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A lost node under one-replica workloads pinned to it is one incident.
// The replacements the taint manager creates stay Pending on the dead
// node; they are the node's symptom, so no workload opens an incident
// of its own, even a quiet roll-up one.
func TestLostNodeUnderPinnedTenantsIsOneIncident(t *testing.T) {
	result := replayNamed(t, "node-lost-pinned-tenants")

	require.NotEmpty(t, result.Incidents)
	for _, inc := range result.Incidents {
		assert.Equal(t, "node//n1", inc.Root.String(),
			"incident %s is rooted elsewhere", inc.ID)
	}
}

// Pods that overcame a start-up FailedMount and then lose their node must
// not have the old mount event revived as a volume failure: the only
// incident is the node.
func TestLostNodeAfterMountRaceIsOneIncident(t *testing.T) {
	result := replayNamed(t, "node-lost-after-mount-race")

	require.NotEmpty(t, result.Incidents)
	for _, inc := range result.Incidents {
		assert.Equal(t, "node//n1", inc.Root.String(),
			"incident %s is rooted elsewhere", inc.ID)
	}
}
