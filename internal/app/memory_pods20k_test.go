package app

import (
	"os"
	"runtime"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// memPods20k is the cluster size of the sizing guidance in
// docs/production-operations.md.
const memPods20k = 20000

// Budgets for 20,000 realistic pods, set from the measured result with
// about 25% headroom (see the test log for the measured values).
const (
	// mem20kCacheMiB bounds the informer cache after the transform.
	mem20kCacheMiB = 200
	// mem20kTotalMiB bounds cache and model together.
	mem20kTotalMiB = 650
	// memCacheShrink is the least the transform must save compared with
	// caching the pods as the API server sent them.
	memCacheShrink = 0.75
)

func heapNow() uint64 {
	runtime.GC()
	runtime.GC()
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	return stats.HeapAlloc
}

// cachePods builds n realistic pods and keeps what an informer would
// cache. With trim false it applies only what the transform did before
// the pod trim: the managedFields trim and dropping the last-applied
// annotation.
func cachePods(t *testing.T, n int, now time.Time, trim bool) []any {
	t.Helper()
	transform := kube.InformerTransform([]byte("memory-test-key"))
	out := make([]any, 0, n)
	for i := 0; i < n; i++ {
		var obj any = realisticPod(i, now)
		var err error
		if trim {
			obj, err = transform(obj)
		} else {
			obj, err = kube.TrimToLatestManager(obj)
			delete(obj.(*corev1.Pod).Annotations,
				"kubectl.kubernetes.io/last-applied-configuration")
		}
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, obj)
	}
	return out
}

// TestMemoryBudgetTwentyThousandPods caches 20,000 realistic pods
// through the informer transform, loads them into the model, and checks
// the heap against the sizing guidance. It is slow, so it runs only when
// KWATCH_MEMORY_20K is set; make verify does not set it.
func TestMemoryBudgetTwentyThousandPods(t *testing.T) {
	if os.Getenv("KWATCH_MEMORY_20K") == "" {
		t.Skip("set KWATCH_MEMORY_20K=1 to run the 20,000-pod measurement")
	}
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	base := heapNow()
	untrimmed := cachePods(t, memPods20k, now, false)
	before := heapNow() - base
	runtime.KeepAlive(untrimmed)
	base = heapNow()

	pods := cachePods(t, memPods20k, now, true)
	cache := heapNow() - base
	t.Logf("cache before trim: %.1f MiB (%.0f bytes/pod)",
		mib(before), float64(before)/memPods20k)
	t.Logf("cache after trim:  %.1f MiB (%.0f bytes/pod)",
		mib(cache), float64(cache)/memPods20k)

	model := inventory.NewModel(inventory.Options{
		EnrichmentSources: kube.EnrichmentSources(),
	})
	translator := kube.NewTranslator(kube.PodSchema{})
	for _, obj := range pods {
		for _, o := range translator.Added(obj, true, now) {
			if _, err := model.Apply(o); err != nil {
				t.Fatalf("apply %s: %v", o.Entity, err)
			}
		}
	}
	total := heapNow() - base
	t.Logf("model: %.1f MiB; cache and model: %.1f MiB (was %.1f MiB)",
		mib(total-cache), mib(total), mib(total-cache+before))
	runtime.KeepAlive(pods)
	runtime.KeepAlive(model)

	if float64(cache) > float64(before)*memCacheShrink {
		t.Errorf("cache %.1f MiB is not under %.0f%% of the untrimmed %.1f MiB",
			mib(cache), memCacheShrink*100, mib(before))
	}
	if mib(cache) > mem20kCacheMiB {
		t.Errorf("cache %.1f MiB exceeds %d MiB", mib(cache), mem20kCacheMiB)
	}
	if mib(total) > mem20kTotalMiB {
		t.Errorf("heap %.1f MiB exceeds %d MiB", mib(total), mem20kTotalMiB)
	}
}
