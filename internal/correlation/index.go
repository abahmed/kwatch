package correlation

import (
	"sort"

	"github.com/abahmed/kwatch/internal/model"
)

// The engine keeps three indices over e.state, all maintained by the two
// functions below.
//
// Every one of them replaced a full scan of every incident in the cluster,
// taken while holding the engine lock. Those scans ran on paths that fire
// constantly: an observed recovery (once per healthy object per resync), the
// node-inhibition refresh (on every node incident change) and the
// owner-baseline sweep. On a cluster with a few thousand objects a resync
// burst spent most of its time walking a map to find the handful of
// incidents that could possibly match, with every other producer blocked
// behind it.
//
// The invariant is simple and worth stating because breaking it is silent:
// an incident is in the indices exactly while it is in e.state under the same
// key. Add through indexIncident, remove through unindexIncident, and never
// touch e.state directly.

// indexIncident records an incident in every index it belongs to. Caller must
// hold e.mu.
func (e *Engine) indexIncident(inc *model.Incident) {
	if inc == nil {
		return
	}
	// The subject reference must not move while the incident is indexed, or
	// unindexIncident looks in the wrong bucket and leaves a dangling entry.
	// Incidents built by this package already froze it; one restored from an
	// older on-disk state, or assembled by hand, may not have, and its
	// display name is rewritten as replicas are replaced.
	if inc.Object.Name == "" {
		inc.SetObject()
	}
	e.indexIncidentByNamespace(inc)
	if ref := inc.Ref(); ref.Name != "" {
		if e.subjectIndex[ref] == nil {
			e.subjectIndex[ref] = make(map[model.IncidentKey]struct{})
		}
		e.subjectIndex[ref][inc.Key] = struct{}{}
		if inc.Resource == "node" {
			if e.nodeIncidents[ref.Name] == nil {
				e.nodeIncidents[ref.Name] = make(
					map[model.IncidentKey]struct{},
				)
			}
			e.nodeIncidents[ref.Name][inc.Key] = struct{}{}
		}
	}
}

// unindexIncident removes an incident from every index. Caller must hold
// e.mu.
func (e *Engine) unindexIncident(inc *model.Incident) {
	if inc == nil {
		return
	}
	e.removeIncidentFromNamespaceIndex(inc)
	ref := inc.Ref()
	if ref.Name == "" {
		return
	}
	delete(e.subjectIndex[ref], inc.Key)
	if len(e.subjectIndex[ref]) == 0 {
		delete(e.subjectIndex, ref)
	}
	if inc.Resource != "node" {
		return
	}
	delete(e.nodeIncidents[ref.Name], inc.Key)
	if len(e.nodeIncidents[ref.Name]) == 0 {
		delete(e.nodeIncidents, ref.Name)
	}
}

// incidentsForSubject returns the live incidents about one object, in a
// stable key order. Caller must hold e.mu.
func (e *Engine) incidentsForSubject(
	ref model.ObjectRef,
) []model.IncidentKey {
	bucket := e.subjectIndex[ref]
	if len(bucket) == 0 {
		return nil
	}
	keys := make([]model.IncidentKey, 0, len(bucket))
	for key := range bucket {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(a, b int) bool { return keys[a] < keys[b] })
	return keys
}

// nodeIncidentKeys returns the incidents about one node, in a stable key
// order. Caller must hold e.mu.
func (e *Engine) nodeIncidentKeys(nodeName string) []model.IncidentKey {
	bucket := e.nodeIncidents[nodeName]
	if len(bucket) == 0 {
		return nil
	}
	keys := make([]model.IncidentKey, 0, len(bucket))
	for key := range bucket {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(a, b int) bool { return keys[a] < keys[b] })
	return keys
}

// baselineOwnerBucket is the index key for the owner-level baseline: the
// "namespace:owner:" prefix every key of one owner shares.
func baselineOwnerBucket(namespace, owner string) string {
	return namespace + ":" + owner + ":"
}

// indexBaselineKey records a baseline key under its owner bucket, so
// clearOwnerBaseline can find one owner's entries without walking every
// baseline key in the cluster. Caller must hold e.mu.
func (e *Engine) indexBaselineKey(key string) {
	pk := ParseKey(model.IncidentKey(key))
	if pk.Owner == "" {
		return
	}
	bucket := baselineOwnerBucket(pk.Namespace, pk.Owner)
	if e.baselineByOwner[bucket] == nil {
		e.baselineByOwner[bucket] = make(map[string]struct{})
	}
	e.baselineByOwner[bucket][key] = struct{}{}
}

// unindexBaselineKey forgets a baseline key that no longer exists. Caller
// must hold e.mu.
func (e *Engine) unindexBaselineKey(key string) {
	pk := ParseKey(model.IncidentKey(key))
	if pk.Owner == "" {
		return
	}
	bucket := baselineOwnerBucket(pk.Namespace, pk.Owner)
	delete(e.baselineByOwner[bucket], key)
	if len(e.baselineByOwner[bucket]) == 0 {
		delete(e.baselineByOwner, bucket)
	}
}

// baselineBucket returns the pod map for a baseline key, creating and
// indexing it on first use. Caller must hold e.mu.
func (e *Engine) baselineBucket(key string) map[string]int64 {
	pods, ok := e.baseline[key]
	if !ok {
		pods = map[string]int64{}
		e.baseline[key] = pods
		e.indexBaselineKey(key)
	}
	return pods
}

// dropEmptyBaselineKey removes a baseline key once its last pod entry is
// gone. Caller must hold e.mu.
func (e *Engine) dropEmptyBaselineKey(key string) {
	if len(e.baseline[key]) > 0 {
		return
	}
	e.dropBaselineKey(key)
}

// dropBaselineKey removes a baseline key and its index entry outright.
// Caller must hold e.mu.
func (e *Engine) dropBaselineKey(key string) {
	delete(e.baseline, key)
	e.unindexBaselineKey(key)
}

// ownerBaselineKeys returns the baseline keys recorded for one owner, in a
// stable order. Caller must hold e.mu.
func (e *Engine) ownerBaselineKeys(namespace, owner string) []string {
	bucket := e.baselineByOwner[baselineOwnerBucket(namespace, owner)]
	if len(bucket) == 0 {
		return nil
	}
	keys := make([]string, 0, len(bucket))
	for key := range bucket {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
