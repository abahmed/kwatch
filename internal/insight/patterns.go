package insight

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/clock"
	context "github.com/abahmed/kwatch/internal/graphcontext"
	"github.com/abahmed/kwatch/internal/model"
)

type MassFailure struct {
	SharedDependency string
	AffectedCount    int
	Threshold        int
	Reason           string
	Namespace        string
	ResourceKind     string
	RootCause        string
	RecentChanges    []context.Change
}

const (
	minMassFailThreshold = 3
)

func dynamicThreshold(depKey string, graph *context.ResourceGraph) int {
	if graph == nil {
		return minMassFailThreshold
	}

	if strings.HasPrefix(depKey, "node/") {
		deps := graph.DependentsByType("node", "", depKey[6:], "pod")
		n := distinctOwnerCount(graph, deps)
		if n >= 3 {
			t := n * 30 / 100
			if t < minMassFailThreshold {
				t = minMassFailThreshold
			}
			return t
		}
		return minMassFailThreshold
	}

	if strings.HasPrefix(depKey, "configmap/") ||
		strings.HasPrefix(depKey, "secret/") ||
		strings.HasPrefix(depKey, "pvc/") {
		if ref, ok := model.ParseObjectKey(depKey); ok {
			kind := ref.Kind
			ns := ref.Namespace
			refs := graph.DependentsOf(kind, ns, ref.Name)
			n := distinctOwnerCount(graph, refs)
			if n >= 3 {
				t := n * 30 / 100
				if t < minMassFailThreshold {
					t = minMassFailThreshold
				}
				return t
			}
		}
		return minMassFailThreshold
	}

	return minMassFailThreshold
}

func ScanMassFailures(
	incidents []*model.Incident,
	graph *context.ResourceGraph,
) []MassFailure {
	if graph == nil {
		return nil
	}
	type depEntry struct {
		owners map[string]bool
		inc    *model.Incident
	}

	shared := make(map[string]*depEntry)

	// Walk the incidents in a stable order. The entry kept per dependency is
	// whichever incident arrived first, and it supplies the reason,
	// namespace and kind the mass-failure alert prints; taken in map order
	// those three could change from tick to tick while nothing about the
	// failure had, so the same outage re-rendered with different wording.
	ordered := make([]*model.Incident, 0, len(incidents))
	ordered = append(ordered, incidents...)
	sort.Slice(ordered, func(a, b int) bool {
		return ordered[a].Key < ordered[b].Key
	})

	// Count distinct workloads, not incidents. Three replicas of one
	// Deployment -- or its CPU, memory and readiness incidents -- all share
	// that Deployment's ServiceAccount, Secret and ReplicaSet by definition;
	// that is one workload having a bad day, not a dependency taking several
	// down.
	for _, inc := range ordered {
		if inc.State != model.StateActive {
			continue
		}
		owner := incidentSubjectKey(inc)
		deps := dependenciesFor(graph, inc)
		seen := make(map[string]bool)
		for _, d := range deps {
			if seen[d] {
				continue
			}
			seen[d] = true
			if shared[d] == nil {
				shared[d] = &depEntry{inc: inc, owners: map[string]bool{}}
			}
			shared[d].owners[owner] = true
		}
	}

	depKeys := make([]string, 0, len(shared))
	for depKey := range shared {
		depKeys = append(depKeys, depKey)
	}
	sort.Strings(depKeys)

	var results []MassFailure
	for _, depKey := range depKeys {
		entry := shared[depKey]
		count := len(entry.owners)
		if count < minMassFailThreshold {
			continue
		}
		th := dynamicThreshold(depKey, graph)
		if count < th {
			continue
		}
		results = append(results, MassFailure{
			SharedDependency: depKey,
			AffectedCount:    count,
			Threshold:        th,
			Reason:           entry.inc.Reason,
			Namespace:        entry.inc.Namespace,
			ResourceKind:     entry.inc.Resource,
		})
	}
	return results
}

// describeDependency renders a graph key for a reader: "node ip-10-0-57-202"
// or "configmap staging/app-settings". Cutting at the first slash used to
// leave cluster-scoped keys as "/ip-10-0-57-202", and "threshold: 4,
// affected: 6" was the detector's arithmetic, not anything to act on.
func describeDependency(depKey string) string {
	ref, ok := model.ParseObjectKey(depKey)
	if !ok {
		return depKey
	}
	return ref.Describe()
}

// workloadKinds are the owners a pod is counted under when sizing a
// threshold. Anything else (a bare pod, a node) counts as itself.
var thresholdWorkloadKinds = map[string]bool{
	"deployment":  true,
	"statefulset": true,
	"daemonset":   true,
	"replicaset":  true,
	"job":         true,
	"cronjob":     true,
}

// distinctOwnerCount collapses dependent pods onto the workloads that own
// them, so a threshold is expressed in the same unit as the affected count.
func distinctOwnerCount(graph *context.ResourceGraph, deps []string) int {
	owners := make(map[string]bool, len(deps))
	for _, dep := range deps {
		ref, ok := model.ParseObjectKey(dep)
		if !ok || ref.Kind != "pod" {
			owners[dep] = true
			continue
		}
		owner := ""
		for _, up := range graph.DependenciesOf(
			ref.Kind,
			ref.Namespace,
			ref.Name,
		) {
			upRef, upOK := model.ParseObjectKey(up)
			if upOK && thresholdWorkloadKinds[upRef.Kind] {
				owner = up
				break
			}
		}
		if owner == "" {
			owner = dep
		}
		owners[owner] = true
	}
	return len(owners)
}

// incidentSubjectKey identifies the workload an incident is about, so several
// incidents on the same subject count once. Pod incidents carry their owning
// workload in Name; everything else is already "namespace/name" or a node.
func incidentSubjectKey(inc *model.Incident) string {
	return inc.Ref().Key()
}

// Describe renders the mass failure for humans, with change ages measured
// against the wall clock.
func (mf MassFailure) Describe() string {
	return mf.describeAt(clock.Now())
}

// describeAt is Describe with an explicit clock, for deterministic tests.
func (mf MassFailure) describeAt(now time.Time) string {
	base := fmt.Sprintf(
		"%d %s workloads share %s",
		mf.AffectedCount,
		mf.ResourceKind,
		describeDependency(mf.SharedDependency),
	)
	if mf.Reason != "" {
		base += " and are all failing with " + mf.Reason
	}
	if mf.RootCause != "" {
		base += "; root cause: " + mf.RootCause
	}
	if len(mf.RecentChanges) > 0 {
		parts := make([]string, 0, len(mf.RecentChanges))
		for _, c := range mf.RecentChanges {
			delta := now.Sub(c.Timestamp).Round(time.Second)
			if delta < 0 {
				delta = 0
			}
			parts = append(parts, fmt.Sprintf("%s/%s updated %s ago",
				c.Namespace, c.Name, delta))
		}
		base += "; recent changes: " + strings.Join(parts, ", ")
	}
	return base
}
