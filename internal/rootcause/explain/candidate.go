package explain

import (
	"sort"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// selfRow is the row of a failing workload blamed for itself.
var selfRow = Row{Name: "self", Prior: SelfPrior}

// summaryRow marks an owner's summary finding covered with its pods.
var summaryRow = Row{Name: "summary"}

// coverage records how a candidate explains one effect.
type coverage struct {
	match rowMatch
	link  LinkType
	chain []inventory.EntityID
	// derived marks an owner's summary covered with its pods; it
	// counts for the set cover but not for the evidence.
	derived bool
}

// candidate is one entity that may explain some failures.
type candidate struct {
	id     inventory.EntityID
	covers map[inventory.EntityID]coverage
	// findings are the candidate's own active findings.
	findings []detection.Finding
	// chain is the failure chain the candidate was extended along.
	chain chainResult
}

// direct returns the effects the candidate explains through a row,
// sorted, without derived summaries.
func (c *candidate) direct() []inventory.EntityID {
	var out []inventory.EntityID
	for effect, how := range c.covers {
		if !how.derived {
			out = append(out, effect)
		}
	}
	sortIDs(out)
	return out
}

// others returns the direct effects other than the candidate itself.
func (c *candidate) others() []inventory.EntityID {
	var out []inventory.EntityID
	for _, effect := range c.direct() {
		if effect != c.id {
			out = append(out, effect)
		}
	}
	return out
}

// isSelf reports whether the candidate only explains failures as
// their own workload.
func (c *candidate) isSelf() bool {
	for _, how := range c.covers {
		if !how.derived && how.match.row.Name != selfRow.Name {
			return false
		}
	}
	return true
}

// selfPrior is the prior of a candidate that only explains itself or
// its own pods. An entity Failing on its own, such as a node that
// stopped reporting, is a stronger cause of itself than a workload
// whose pods crash.
func (c *candidate) selfPrior() float64 {
	for _, f := range c.findings {
		if f.Health == detection.Failing {
			return SelfFailingPrior
		}
	}
	return SelfPrior
}

// bestMatch is the covered row with the highest prior.
func (c *candidate) bestMatch() coverage {
	var best coverage
	for _, effect := range c.direct() {
		how := c.covers[effect]
		if best.chain == nil || how.match.row.Prior > best.match.row.Prior {
			best = how
		}
	}
	return best
}

// candidateSet collects candidates and the reasons others were
// dropped.
type candidateSet struct {
	byID     map[inventory.EntityID]*candidate
	rejected map[inventory.EntityID]rejectNote
	// checked lists, per effect, the upstream entities that showed
	// nothing wrong; see Trace.Checked.
	checked map[inventory.EntityID][]inventory.EntityID
}

// noteChecked records that id, upstream of effect, was reached and
// showed no finding and no change.
func (cs *candidateSet) noteChecked(effect, id inventory.EntityID) {
	if cs.checked == nil {
		cs.checked = map[inventory.EntityID][]inventory.EntityID{}
	}
	cs.checked[effect] = append(cs.checked[effect], id)
}

// rejectNote says why an entity was dropped, and for which failure.
type rejectNote struct {
	reason string
	effect inventory.EntityID
	// insufficient marks a candidate dropped only because too few
	// failures share it; the cause of each failure is not in doubt.
	insufficient bool
}

// add records that id explains effect, keeping the best row.
func (cs *candidateSet) add(
	id, effect inventory.EntityID, how coverage,
) {
	c, ok := cs.byID[id]
	if !ok {
		c = &candidate{id: id, covers: map[inventory.EntityID]coverage{}}
		cs.byID[id] = c
	}
	old, ok := c.covers[effect]
	if !ok || old.derived ||
		(!how.derived && how.match.row.Prior > old.match.row.Prior) {
		c.covers[effect] = how
	}
}

// reject records why an entity is not a candidate, keeping the first.
func (cs *candidateSet) reject(
	id, effect inventory.EntityID, reason string,
) {
	if _, ok := cs.rejected[id]; !ok {
		cs.rejected[id] = rejectNote{reason: reason, effect: effect}
	}
}

// rejectInsufficient records a candidate dropped for want of sharers.
func (cs *candidateSet) rejectInsufficient(
	id, effect inventory.EntityID, reason string,
) {
	if _, ok := cs.rejected[id]; !ok {
		cs.rejected[id] = rejectNote{reason: reason, effect: effect,
			insufficient: true}
	}
}

// candidates walks upstream from every failure and keeps what a row
// links to it. Reachability alone is never a cause: an upstream entity
// that is healthy and unchanged is skipped.
func (v *view) candidates(failures []inventory.EntityID) *candidateSet {
	cs := &candidateSet{byID: map[inventory.EntityID]*candidate{},
		rejected: map[inventory.EntityID]rejectNote{}}
	for _, effect := range failures {
		v.addUpstream(cs, effect)
		v.addSelf(cs, effect)
	}
	applyMinCovered(cs)
	v.applySharedFactorWindow(cs)
	v.applySignatureWindow(cs)
	v.applyMinWorkloads(cs)
	// After pruning, so a summary is never covered through an effect
	// that was dropped.
	v.addSummaries(cs, failures)
	v.addMetricsBackends(cs)
	for id, c := range cs.byID {
		c.findings = v.s.Findings[id]
	}
	return cs
}

// addUpstream adds the candidates upstream of one effect.
func (v *view) addUpstream(cs *candidateSet, effect inventory.EntityID) {
	effectState := v.effectState(effect)
	reached := v.walk(effect)
	v.noteInputs(effect, reached)
	for _, r := range reached {
		cause := v.causeState(r.id, effect, r.link)
		if len(cause.modes) == 0 {
			// Nothing is wrong with it; it is still what several
			// failing workloads may have in common.
			cs.noteChecked(effect, r.id)
			cause = v.sharedFactorState(r.id)
			if len(cause.modes) == 0 {
				continue
			}
		}
		if v.throughHealthyNode(r) {
			cs.reject(r.id, effect, "the node between it and "+
				effect.String()+" is healthy")
			continue
		}
		match, ok := bestRow(propagation, cause, r.link, effectState)
		if !ok {
			cs.reject(r.id, effect, "no propagation row links it to "+
				effect.String())
			continue
		}
		if !v.s.verifiable(r.id.Kind) {
			// What kwatch cannot see may only look missing: name the
			// gap instead of blaming it.
			scope := rootcause.UnverifiedScope(r.id)
			v.unverified[effect] = append(v.unverified[effect], scope)
			cs.reject(r.id, effect, "kwatch cannot see "+scope)
			continue
		}
		// A part of the failure's own workload (an init container, a
		// sidecar) is not timed against it: the pod and its container
		// date their findings differently.
		cs.add(r.id, effect, coverage{match: match, link: r.link,
			chain: r.chain})
	}
}

// throughHealthyNode reports whether a zone or pool reaches its effect
// through a node that is not failing. Such a group explains only what
// runs on its failing nodes: a crash on a working node is not the
// zone's, however many other nodes of the zone are down.
func (v *view) throughHealthyNode(r reach) bool {
	if r.id.Kind != kube.KindZone && r.id.Kind != kube.KindNodePool {
		return false
	}
	// The chain starts at the group and ends at the effect, which may
	// itself be a failing node.
	for _, id := range r.chain[1 : len(r.chain)-1] {
		if id.Kind == kube.KindNode && !v.failing(id) {
			return true
		}
	}
	return false
}

// addSelf makes the failure's own workload a candidate for it, so a
// failure with no better cause is explained by what owns it.
func (v *view) addSelf(cs *candidateSet, effect inventory.EntityID) {
	unit := effect
	if pod, ok := v.podOf(effect); ok {
		unit = pod
	}
	root := rootcause.TopOwner(v.s.Model, unit)
	chain := []inventory.EntityID{root}
	if root != effect {
		chain = append(chain, effect)
	}
	how := coverage{match: rowMatch{row: selfRow}, chain: chain}
	if match, ok := v.selfMatch(root, effect); ok {
		how.match, how.link = match, LinkSelf
	}
	cs.add(root, effect, how)
}

// addSummaries lets a candidate that explains a pod also explain the
// summary findings of the pod and its owners ("2 of 4 replicas are
// ready"): they restate the pod's failure and must not stand alone.
func (v *view) addSummaries(
	cs *candidateSet, failures []inventory.EntityID,
) {
	isFailure := map[inventory.EntityID]bool{}
	for _, f := range failures {
		isFailure[f] = true
	}
	for _, c := range cs.byID {
		for _, effect := range c.direct() {
			for _, owner := range v.summaryOwners(effect) {
				if isFailure[owner] && v.onlySummaries(owner) {
					cs.add(c.id, owner, coverage{derived: true,
						match: rowMatch{row: summaryRow},
						chain: []inventory.EntityID{c.id, owner}})
				}
			}
		}
	}
}

// summaryOwners are the entities whose findings may summarise effect:
// its pod, the pod's owners and the Services the pod backs.
func (v *view) summaryOwners(effect inventory.EntityID) []inventory.EntityID {
	unit := effect
	var out []inventory.EntityID
	if pod, ok := v.podOf(effect); ok && pod != effect {
		unit = pod
		out = append(out, pod)
	}
	if unit.Kind == kube.KindPod {
		out = append(out, v.servicesSelecting(unit)...)
	}
	return append(out, rootcause.OwnerChain(v.s.Model, unit)...)
}

// onlySummaries reports whether every unhealthy finding of id is a
// summary of its pods, or a pod-level restatement of its containers.
func (v *view) onlySummaries(id inventory.EntityID) bool {
	for _, f := range v.s.Findings[id] {
		if !unhealthy(f) {
			continue
		}
		if id.Kind == kube.KindPod || anyModeMatches(summaryModes, f.Mode) {
			continue
		}
		return false
	}
	return true
}

// applyMinCovered drops rows that need more effects than they cover.
// Candidates and effects are visited in order so the first reject note
// is the same on every run.
func applyMinCovered(cs *candidateSet) {
	for _, id := range sortedKeys(cs.byID) {
		c := cs.byID[id]
		counts := map[string]int{}
		for _, how := range c.covers {
			counts[how.match.row.Name]++
		}
		for _, effect := range sortedKeys(c.covers) {
			how := c.covers[effect]
			if need := how.match.row.MinCovered; need > 0 &&
				counts[how.match.row.Name] < need {
				delete(c.covers, effect)
				cs.rejectInsufficient(c.id, effect, "row "+
					how.match.row.Name+" needs more failures than it explains")
			}
		}
		if len(c.covers) == 0 {
			delete(cs.byID, c.id)
		}
	}
}

// sortIDs sorts ids by their canonical key.
func sortIDs(ids []inventory.EntityID) {
	sort.Slice(ids, func(i, j int) bool {
		return ids[i].String() < ids[j].String()
	})
}

// failures lists the entities with a Failing or Degraded finding.
func failures(findings map[inventory.EntityID][]detection.Finding,
) []inventory.EntityID {
	var out []inventory.EntityID
	for id, found := range findings {
		for _, f := range found {
			if unhealthy(f) {
				out = append(out, id)
				break
			}
		}
	}
	sortIDs(out)
	return out
}
