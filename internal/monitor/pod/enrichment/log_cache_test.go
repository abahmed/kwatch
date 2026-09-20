package enrichment

import (
	"fmt"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestLogCacheDoesNotCacheEmptyKeys(t *testing.T) {
	cache := NewLogCache(func() time.Time { return time.Unix(0, 0) })
	fetches := 0
	fetch := func() string {
		fetches++
		return fmt.Sprintf("logs-%d", fetches)
	}

	if got := cache.Do("", fetch); got != "logs-1" {
		t.Fatalf("first fetch = %q, want logs-1", got)
	}
	if got := cache.Do("", fetch); got != "logs-2" {
		t.Fatalf("second fetch = %q, want logs-2", got)
	}
	if fetches != 2 {
		t.Fatalf("fetch count = %d, want 2", fetches)
	}
}

func TestLogCacheExpiresEntries(t *testing.T) {
	now := time.Unix(0, 0)
	cache := NewLogCache(func() time.Time { return now })
	fetches := 0
	fetch := func() string {
		fetches++
		return fmt.Sprintf("logs-%d", fetches)
	}

	if got := cache.Do("pod/container/0", fetch); got != "logs-1" {
		t.Fatalf("initial fetch = %q, want logs-1", got)
	}
	now = now.Add(logCacheTTL - time.Nanosecond)
	if got := cache.Do("pod/container/0", fetch); got != "logs-1" {
		t.Fatalf("cached fetch = %q, want logs-1", got)
	}
	now = now.Add(2 * time.Nanosecond)
	if got := cache.Do("pod/container/0", fetch); got != "logs-2" {
		t.Fatalf("expired fetch = %q, want logs-2", got)
	}
}

func TestLogCacheKeyChangesWithRestartCount(t *testing.T) {
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{UID: "uid-1"}}
	first := &corev1.ContainerStatus{Name: "app", RestartCount: 1}
	second := first.DeepCopy()
	second.RestartCount++

	if got, want := LogCacheKey(pod, first), "uid-1/app/1"; got != want {
		t.Fatalf("first key = %q, want %q", got, want)
	}
	if LogCacheKey(pod, first) == LogCacheKey(pod, second) {
		t.Fatal("restart count did not change the cache key")
	}
}

func TestLogCacheRemainsBoundedAfterCapacityEviction(t *testing.T) {
	now := time.Unix(0, 0)
	cache := NewLogCache(func() time.Time { return now })

	for i := 0; i <= maxLogCacheEntries; i++ {
		key := fmt.Sprintf("pod/container/%d", i)
		cache.Do(key, func() string { return key })
	}

	cache.mu.Lock()
	defer cache.mu.Unlock()
	if len(cache.m) > maxLogCacheEntries {
		t.Fatalf(
			"cache size = %d, exceeds %d",
			len(cache.m), maxLogCacheEntries,
		)
	}
	if _, ok := cache.m["pod/container/0"]; ok {
		t.Fatal("capacity eviction retained the oldest entry")
	}
}
