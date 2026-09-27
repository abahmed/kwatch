package model

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMissingServicePortFactsStayOutOfPersistedFormat(t *testing.T) {
	inc := &Incident{Evidence: Evidence{Facts: Facts{
		MissingServicePortKind:  "target port",
		MissingServicePortValue: "TCP/8080",
		BackendsObserved:        true,
		BackendPods:             3,
		UnreadyBackendPods:      3,
		SharedFailingNode:       "node-a",
		NodeFailureReason:       "KubeletNotReady",
	}}}
	raw, err := json.Marshal(inc.ToPersisted())
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "missingServicePort")
	assert.NotContains(t, string(raw), "node-a")
	assert.NotContains(t, string(raw), "backendPods")
}
