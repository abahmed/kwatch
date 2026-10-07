package detectors

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
)

const awsIPMessage = "Failed to create pod sandbox: rpc error: code = " +
	"Unknown desc = failed to setup network for sandbox: plugin " +
	"type=\"aws-cni\" failed (add): add cmd: failed to assign an IP " +
	"address to container"

func TestSandboxQuoteKeepsTheFailureAndDropsThePrefix(t *testing.T) {
	assert.Equal(t, "failed to assign an IP address to container",
		sandboxQuote(awsIPMessage))
	assert.Equal(t, "failed to allocate for range 0: no IP addresses "+
		"available in range set: 10.244.1.1-10.244.1.254",
		sandboxQuote(ipExhaustedMessage))
	assert.Equal(t, "something else", sandboxQuote("something else"))
}

func TestNodeIPFindingQuotesTheEvent(t *testing.T) {
	model := newTestModel()
	sandboxPods(model, awsIPMessage, awsIPMessage, awsIPMessage)
	var got []detection.Finding
	for _, f := range detectNode(model) {
		if f.Reason == reasons.NodePodIPExhausted {
			got = append(got, f)
		}
	}
	require.Len(t, got, 1)
	quoted := ""
	for _, e := range got[0].Evidence {
		if e.Label == detection.EvidenceSandboxEvent {
			quoted = e.Value
		}
	}
	assert.Equal(t, "failed to assign an IP address to container", quoted)
}
