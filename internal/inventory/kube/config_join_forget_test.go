package kube

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// An endpoint nobody reads any more leaves nothing behind in the join:
// churning pods and ConfigMaps must not grow its memory without end.
func TestConfigJoinForgetsEndpointsNobodyReads(t *testing.T) {
	h := newJoinHarness(t)
	h.addMap(sharedConfigMap())
	reader := readerPod("api-1", keyRef("api-config", "DB_HOST"))
	h.apply(h.pods.Added(reader, true, joinAt))
	assert.Equal(t, []string{"db.example.com:5432"}, h.calls("api-1"))

	h.apply(h.pods.Deleted(reader, joinAt))

	join := h.pods.join
	assert.Empty(t, join.said)
	assert.Empty(t, join.told)
}
