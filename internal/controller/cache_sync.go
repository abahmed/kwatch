package controller

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"k8s.io/client-go/tools/cache"
)

// cacheSyncTimeout bounds the initial informer sync. Without a bound, one
// resource the ServiceAccount cannot list — a missing RBAC rule, an API group
// the cluster does not serve — parks kwatch forever: never ready, never
// alerting, with only reflector errors in the log to explain why.
var cacheSyncTimeout = 5 * time.Minute // a var so tests can shorten it

// waitForCaches waits for every informer to sync and, on timeout, names the
// pipelines that never did so the failure points at the missing permission.
func (c *Controller) waitForCaches(
	ctx context.Context,
	syncFns []cache.InformerSynced,
) error {
	waitCtx, cancel := context.WithTimeout(ctx, cacheSyncTimeout)
	defer cancel()
	if cache.WaitForCacheSync(waitCtx.Done(), syncFns...) {
		return nil
	}
	if ctx.Err() != nil {
		return fmt.Errorf("failed to wait for caches to sync: %w", ctx.Err())
	}
	var unsynced []string
	for _, p := range c.allPipelines() {
		if p == nil {
			continue
		}
		unsynced = appendUnsynced(unsynced, p.name, p.synced)
	}
	unsynced = appendUnsynced(unsynced, "replicaset graph support", c.rsSynced)
	unsynced = appendUnsynced(unsynced, "daemonset graph support", c.dsSynced)
	unsynced = appendUnsynced(unsynced, "statefulset graph support", c.ssSynced)
	unsynced = appendUnsynced(unsynced, "pod events", c.eventsSynced)
	unsynced = appendUnsynced(unsynced, "configmaps", c.configMapSynced)
	unsynced = appendUnsynced(unsynced, "TLS secrets", c.secretsSynced)
	unsynced = appendUnsynced(unsynced, "graph support", c.graphSynced)
	sort.Strings(unsynced)
	return fmt.Errorf(
		"informer caches did not sync within %s (unsynced: %s) — check that "+
			"kwatch's ClusterRole grants list/watch on these resources and "+
			"that the API groups exist on this cluster",
		cacheSyncTimeout,
		strings.Join(unsynced, ", "),
	)
}

func appendUnsynced(
	unsynced []string,
	name string,
	syncFns []cache.InformerSynced,
) []string {
	for _, synced := range syncFns {
		if !synced() {
			return append(unsynced, name)
		}
	}
	return unsynced
}
