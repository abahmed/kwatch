package controller

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/cache"
)

func TestWaitForCachesNamesTheInformerThatNeverSynced(t *testing.T) {
	c := &Controller{
		pod:  newResourcePipeline("pod", "pods"),
		node: newResourcePipeline("node", "nodes"),
	}
	c.pod.synced = []cache.InformerSynced{func() bool { return true }}
	c.node.synced = []cache.InformerSynced{func() bool { return false }}

	// Shorten the sync budget itself; a parent deadline would be reported as
	// a shutdown, not as an informer that never synced.
	prev := cacheSyncTimeout
	cacheSyncTimeout = 50 * time.Millisecond
	defer func() { cacheSyncTimeout = prev }()

	err := c.waitForCaches(
		context.Background(),
		append(c.pod.synced, c.node.synced...),
	)
	require.Error(t, err)
	assert.Contains(
		t,
		err.Error(),
		"node",
		"the unsynced pipeline must be named",
	)
	assert.NotContains(
		t,
		err.Error(),
		"unsynced: pod",
		"a synced pipeline must not be blamed",
	)
}
