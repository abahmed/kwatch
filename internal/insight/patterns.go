package insight

import (
	"fmt"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/context"
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
		n := len(deps)
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
		parts := strings.SplitN(depKey, "/", 3)
		if len(parts) == 3 {
			kind := parts[0]
			ns := parts[1]
			refs := graph.DependentsOf(kind, ns, parts[2])
			n := len(refs)
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
		count int
		inc   *model.Incident
	}

	shared := make(map[string]*depEntry)

	for _, inc := range incidents {
		if inc.State != model.StateActive {
			continue
		}
		deps := dependenciesFor(graph, inc)
		seen := make(map[string]bool)
		for _, d := range deps {
			if seen[d] {
				continue
			}
			seen[d] = true
			if shared[d] == nil {
				shared[d] = &depEntry{inc: inc}
			}
			shared[d].count++
		}
	}

	var results []MassFailure
	for depKey, entry := range shared {
		if entry.count < minMassFailThreshold {
			continue
		}
		th := dynamicThreshold(depKey, graph)
		if entry.count < th {
			continue
		}
		results = append(results, MassFailure{
			SharedDependency: depKey,
			AffectedCount:    entry.count,
			Threshold:        th,
			Reason:           entry.inc.Reason,
			Namespace:        entry.inc.Namespace,
			ResourceKind:     entry.inc.Resource,
		})
	}
	return results
}

// Describe renders the mass failure for humans, with change ages measured
// against the wall clock.
func (mf MassFailure) Describe() string {
	return mf.describeAt(time.Now())
}

// describeAt is Describe with an explicit clock, for deterministic tests.
func (mf MassFailure) describeAt(now time.Time) string {
	short := mf.SharedDependency
	if idx := strings.Index(short, "/"); idx >= 0 {
		short = short[idx+1:]
	}
	base := fmt.Sprintf(
		"%d %s incidents share dependency %s (threshold: %d, affected: %d)",
		mf.AffectedCount,
		mf.ResourceKind,
		short,
		mf.Threshold,
		mf.AffectedCount,
	)
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
