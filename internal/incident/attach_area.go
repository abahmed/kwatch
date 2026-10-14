package incident

import (
	"slices"
	"strconv"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/rootcause"
	"github.com/abahmed/kwatch/internal/rootcause/explain"
)

// areaPlacer places the failures of one area. It indexes which cause
// covers each failure and remembers each cause's record.
type areaPlacer struct {
	s    explain.Snapshot
	area explain.Area
	// covering maps a failure to the index of the first cause that
	// covers it.
	covering map[inventory.EntityID]int
	records  map[int]rootcause.CauseRecord
	blamed   map[int]bool
	// outside caches, per own root, whether a rejected candidate lies
	// outside that workload.
	outside map[inventory.EntityID]bool
}

func newAreaPlacer(s explain.Snapshot, area explain.Area) *areaPlacer {
	covering := map[inventory.EntityID]int{}
	for i, cause := range area.Causes {
		for _, covered := range cause.Covers {
			if _, seen := covering[covered]; !seen {
				covering[covered] = i
			}
		}
	}
	return &areaPlacer{s: s, area: area, covering: covering,
		records: map[int]rootcause.CauseRecord{}, blamed: map[int]bool{},
		outside: map[inventory.EntityID]bool{}}
}

// placementOf finds the incident a failure of the area belongs to. A
// failure no stated cause covers belongs to its own workload, without
// a cause. So does a failure whose cause stays inside its own workload
// (a Deployment explained by its crashing pods), unless a change of
// that workload is blamed or the row names a fault of the workload
// itself (a memory limit, a failing sidecar: explain.InsideRow), and a
// failure blamed on itself by the self row: "it broke on its own" names
// no cause, whatever changed. A failure left without a cause while
// candidates outside its workload were dropped has an unclear cause.
func (a *areaPlacer) placementOf(failure inventory.EntityID) placement {
	own := defaultRoot(a.s.Model, failure)
	out := placement{root: own, unverified: a.area.Unverified,
		checked:    checkedKinds(a.area.Trace.Checked[failure]),
		considered: consideredCauses(a.area.Alternatives)}
	i, ok := a.covering[failure]
	if !ok {
		out.unclear = a.rejectedOutside(own)
		return out
	}
	cause := a.area.Causes[i]
	root := defaultRoot(a.s.Model, cause.Root)
	if root == own && (cause.Row == selfRow || (!a.blamesChange(i) &&
		!explain.InsideRow(cause.Row))) {
		out.unclear = a.rejectedOutside(own)
		return out
	}
	record := a.record(i)
	out.root, out.cause = root, &record
	return out
}

// maxConsidered bounds the alternatives an incident remembers.
const maxConsidered = 3

// consideredCauses words the alternatives the solver weighed and did
// not choose, best first, for the audit log: "node//n1 (node-not-ready,
// 0.42)". They let a reader see what the engine ruled out.
func consideredCauses(alternatives []explain.Cause) []string {
	var out []string
	for i, c := range alternatives {
		if i == maxConsidered {
			break
		}
		out = append(out, c.Root.String()+" ("+c.Row+", "+
			strconv.FormatFloat(c.Confidence, 'f', 2, 64)+")")
	}
	return out
}

// checkedKinds names the kinds of the checked entities, sorted and
// without repeats.
func checkedKinds(ids []inventory.EntityID) []string {
	var out []string
	for _, id := range ids {
		out = mergeSorted(out, string(id.Kind))
	}
	return out
}

// mergeSorted inserts value into the sorted list unless it is there.
func mergeSorted(sorted []string, value string) []string {
	i, found := slices.BinarySearch(sorted, value)
	if found {
		return sorted
	}
	return slices.Insert(sorted, i, value)
}

// rejectedOutside reports whether the area dropped a candidate that is
// not part of own's workload, computed once per own root. A rejection
// of the workload itself or of its pods says nothing about an upstream
// cause.
func (a *areaPlacer) rejectedOutside(own inventory.EntityID) bool {
	outside, ok := a.outside[own]
	if ok {
		return outside
	}
	for _, r := range a.area.Trace.Rejected {
		if !r.Insufficient && defaultRoot(a.s.Model, r.Root) != own {
			outside = true
			break
		}
	}
	a.outside[own] = outside
	return outside
}

// blamesChange reports whether cause i blames a change, computed once.
func (a *areaPlacer) blamesChange(i int) bool {
	blamed, ok := a.blamed[i]
	if !ok {
		blamed = len(a.s.BlamedChanges(a.area.Causes[i])) > 0
		a.blamed[i] = blamed
	}
	return blamed
}

// record is the incident record of cause i, built once. Each caller
// gets its own copy.
func (a *areaPlacer) record(i int) rootcause.CauseRecord {
	record, ok := a.records[i]
	if !ok {
		record = a.s.Record(a.area.Causes[i])
		record.Unverified = a.area.Unverified
		a.annotate(&record, a.area.Causes[i])
		a.records[i] = record
	}
	return record
}
