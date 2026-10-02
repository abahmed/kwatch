package app

import (
	"runtime"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// memBudgetMiB is the most live heap the 5,000-pod cluster may keep
// after a collection (docs/production-goals.md). The default 512Mi
// limit sets GOMEMLIMIT at about 90%; the rest of the limit is stacks,
// runtime overhead and the delivery queues.
const memBudgetMiB = 250

// memPeakMiB bounds the peak heap: the most heap memory the process
// held from the operating system at any sampled phase, live objects and
// garbage not yet collected together. It is what counts against the
// container limit, so it may not pass the default 512Mi.
const memPeakMiB = 512

// memKinds are the kinds detection evaluates in the budget test.
var memKinds = []inventory.Kind{
	kube.KindNamespace, kube.KindNode, kube.KindDeployment,
	kube.KindReplicaSet, kube.KindStatefulSet, kube.KindDaemonSet,
	kube.KindPod, kube.KindContainer, kube.KindService,
	kube.KindEndpointSlice, kube.KindSecret, kube.KindConfigMap,
}

// heapSampler records the largest heap seen at phase boundaries: peak
// is the largest allocated heap, held the largest heap memory obtained
// from the OS and not yet returned (HeapSys - HeapReleased).
type heapSampler struct {
	t        *testing.T
	peak     uint64
	held     uint64
	baseline uint64
}

func (s *heapSampler) sample(phase string) {
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	s.peak = max(s.peak, stats.HeapAlloc)
	s.held = max(s.held, stats.HeapSys-stats.HeapReleased)
	s.t.Logf("%-10s heapAlloc=%.1f MiB heapInuse=%.1f MiB", phase,
		mib(stats.HeapAlloc), mib(stats.HeapInuse))
}

func mib(bytes uint64) float64 { return float64(bytes) / (1 << 20) }

// TestMemoryBudgetLargeCluster loads a 5,000-pod cluster through the
// real translators, runs every detector and one root-cause solve, and
// checks the live heap stays within the budget the chart's default
// memory limit allows.
func TestMemoryBudgetLargeCluster(t *testing.T) {
	if testing.Short() {
		t.Skip("memory budget test builds a large cluster")
	}
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	runtime.GC()
	sampler := &heapSampler{t: t}
	var start runtime.MemStats
	runtime.ReadMemStats(&start)
	sampler.baseline = start.HeapAlloc

	model := inventory.NewModel(inventory.Options{
		EnrichmentSources: kube.EnrichmentSources(),
	})
	// The generated objects go out of scope once translated: the model
	// is what the measurement is about.
	loadMemCluster(t, model, buildMemCluster(now), now)
	sampler.sample("load")

	synced := func(inventory.Kind) bool { return true }
	findings := detectAll(model, newDetectorRegistry(synced), now)
	sampler.sample("detect")

	snapshot := explain.NewSnapshot(model, findings, synced, now)
	areas := explain.NewSolver().Solve(snapshot, failingIDs(findings))
	sampler.sample("solve")

	runtime.GC()
	var end runtime.MemStats
	runtime.ReadMemStats(&end)
	stats := model.Stats()
	t.Logf("entities=%d relations=%d findings=%d areas=%d",
		stats.Entities, stats.Relations, len(findings), len(areas))
	t.Logf("live heap after GC: %.1f MiB (in use %.1f MiB, "+
		"model share %.1f MiB); peak sampled %.1f MiB allocated, "+
		"%.1f MiB held", mib(end.HeapAlloc), mib(end.HeapInuse),
		mib(end.HeapAlloc-min(end.HeapAlloc, sampler.baseline)),
		mib(sampler.peak), mib(sampler.held))

	if stats.Entities < memDeployments*memDeployPods {
		t.Fatalf("model has %d entities, the cluster did not load",
			stats.Entities)
	}
	if len(areas) == 0 {
		t.Fatal("the crash-looping pods produced no solved area")
	}
	if got := mib(end.HeapAlloc); got > memBudgetMiB {
		t.Fatalf("live heap %.1f MiB exceeds the %d MiB budget",
			got, memBudgetMiB)
	}
	if got := mib(sampler.held); got > memPeakMiB {
		t.Fatalf("peak heap %.1f MiB exceeds the %d MiB limit",
			got, memPeakMiB)
	}
	runtime.KeepAlive(model)
	runtime.KeepAlive(findings)
	runtime.KeepAlive(areas)
}

// loadMemCluster feeds every object through its kind's translator as an
// initial list, the way the typed source does.
func loadMemCluster(
	t *testing.T, model *inventory.Model, c memCluster, now time.Time,
) {
	t.Helper()
	digester := kube.NewDigester([]byte("memory-budget-test-key"))
	for _, secret := range c.secrets {
		if _, err := digester.HashSecretData(secret); err != nil {
			t.Fatal(err)
		}
	}
	for _, cm := range c.configMaps {
		if _, err := digester.HashConfigMapData(cm); err != nil {
			t.Fatal(err)
		}
	}
	groups := []struct {
		schema  kube.Schema
		objects []any
	}{
		{kube.NamespaceSchema{}, c.namespaces},
		{kube.NodeSchema{}, c.nodes},
		{kube.DeploymentSchema(), c.deployments},
		{kube.ReplicaSetSchema(), c.replicaSets},
		{kube.StatefulSetSchema(), c.statefulSets},
		{kube.DaemonSetSchema(), c.daemonSets},
		{kube.SecretSchema{}, c.secrets},
		{kube.ConfigMapSchema{}, c.configMaps},
		{kube.ServiceSchema{}, c.services},
		{kube.EndpointSliceSchema{}, c.slices},
		{kube.PodSchema{}, c.pods},
	}
	for _, group := range groups {
		translator := kube.NewTranslator(group.schema)
		for _, obj := range group.objects {
			for _, o := range translator.Added(obj, true, now) {
				if _, err := model.Apply(o); err != nil {
					t.Fatalf("apply %s: %v", o.Entity, err)
				}
			}
		}
	}
}

// detectAll evaluates every entity once and keeps those with findings.
func detectAll(
	model *inventory.Model, registry *detection.Registry, now time.Time,
) map[inventory.EntityID][]detection.Finding {
	out := map[inventory.EntityID][]detection.Finding{}
	for _, kind := range memKinds {
		for _, id := range model.Entities(kind) {
			if found := registry.Evaluate(model, now, id).Findings; len(
				found) > 0 {
				out[id] = found
			}
		}
	}
	return out
}

// failingIDs lists the entities with findings: everything is dirty on
// the first solve.
func failingIDs(
	findings map[inventory.EntityID][]detection.Finding,
) []inventory.EntityID {
	ids := make([]inventory.EntityID, 0, len(findings))
	for id := range findings {
		ids = append(ids, id)
	}
	return ids
}
