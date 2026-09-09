package filter

import (
	"strconv"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
)

// logCacheTTL is how long a container's log tail stays reusable.
//
// A crash-looping container is re-reported on every resync, and every report
// used to re-read its tail from the kubelet -- two reads, when the previous
// container had already been collected. In a namespace with a handful of
// crash loops that was the bulk of the API traffic kwatch generated, all of
// it fetching output that had not changed. The cache key carries the restart
// count, so a container that crashes again is read again immediately; the TTL
// only bounds how long an unchanged tail is kept.
const logCacheTTL = 10 * time.Minute

// maxLogCacheEntries bounds the cache. It is a cache of text, so the cap is
// on entries rather than bytes; each holds at most MaxRecentLogLines lines.
const maxLogCacheEntries = 2000

type logCacheEntry struct {
	logs      string
	expiresAt time.Time
}

// LogCache memoises container log tails by pod UID, container and restart
// count -- the three things that decide whether the tail can have changed.
type LogCache struct {
	mu  sync.Mutex
	now func() time.Time
	m   map[string]logCacheEntry
}

// NewLogCache builds a cache reading the given clock. A nil clock uses the
// wall clock.
func NewLogCache(now func() time.Time) *LogCache {
	if now == nil {
		now = time.Now
	}
	return &LogCache{now: now, m: map[string]logCacheEntry{}}
}

// LogCacheKey identifies one container's log tail. It is empty when the pod
// has no UID, which is the case only for objects kwatch built itself; those
// are not cached rather than being cached under a colliding key.
func LogCacheKey(pod *corev1.Pod, container *corev1.ContainerStatus) string {
	if pod == nil || container == nil || pod.UID == "" {
		return ""
	}
	return string(pod.UID) + "/" + container.Name + "/" +
		strconv.Itoa(int(container.RestartCount))
}

// Do returns the tail for key, calling fetch only when the cache cannot
// answer. An empty key is never cached.
func (c *LogCache) Do(key string, fetch func() string) string {
	if c == nil || key == "" {
		return fetch()
	}
	now := c.now()
	c.mu.Lock()
	entry, ok := c.m[key]
	c.mu.Unlock()
	if ok && now.Before(entry.expiresAt) {
		return entry.logs
	}

	logs := fetch()

	c.mu.Lock()
	c.evictLocked(now)
	c.m[key] = logCacheEntry{logs: logs, expiresAt: now.Add(logCacheTTL)}
	c.mu.Unlock()
	return logs
}

// evictLocked drops expired entries, and empties the cache outright if it is
// still over the cap. A bounded cache that cannot be emptied is a memory leak
// with extra steps, and a cold cache costs one extra read per container.
// Caller must hold c.mu.
func (c *LogCache) evictLocked(now time.Time) {
	for key, entry := range c.m {
		if !now.Before(entry.expiresAt) {
			delete(c.m, key)
		}
	}
	if len(c.m) < maxLogCacheEntries {
		return
	}
	clear(c.m)
}
