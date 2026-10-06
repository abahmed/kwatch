package kube

import (
	"context"
	"sync"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
)

// statsFailuresBeforeClear is how many polls in a row may fail before the
// readings of a node and its pods are cleared. One or two missed polls are
// a blip and keep the last reading; more means the numbers are stale.
const statsFailuresBeforeClear = 3

// published names one entity attribute set a source wrote.
type published struct {
	entity inventory.EntityID
	source string
}

// publishLog remembers, per node, which entities its kubelet readings were
// written to. A reading that is no longer reported, or a kubelet that stops
// answering, must remove those attributes: the model only replaces a
// source's attributes when that source observes the entity again.
//
// An entity can be reported by two nodes in one moment (a pod or claim that
// moved). owners names the node that reported it last, and only that node
// may clear it, so the old node never wipes the new node's reading.
type publishLog struct {
	mu sync.Mutex
	// nodes holds, per node, each published key and how many polls in a
	// row an optional source has gone without it.
	nodes  map[string]map[published]int
	owners map[published]string
	missed map[string]int
}

func newPublishLog() *publishLog {
	return &publishLog{
		nodes:  map[string]map[published]int{},
		owners: map[published]string{},
		missed: map[string]int{},
	}
}

// optionalSource reports sources read from endpoints that may fail on
// their own (cAdvisor, the kubelet's /metrics). Their readings get the
// same grace as a failed summary before they are cleared.
func optionalSource(source string) bool {
	return source == throttleSource || source == runtimeSource
}

// answered stores what a successful poll of node published and returns
// empty observations for what the previous poll published and this one
// did not.
func (l *publishLog) answered(
	node string, observations []inventory.Observation, now time.Time,
) []inventory.Observation {
	current := map[published]int{}
	for _, o := range observations {
		if o.Kind == inventory.Observed && len(o.Attributes) > 0 &&
			o.Source != memorySource {
			current[published{o.Entity, o.Source}] = 0
		}
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	previous := l.nodes[node]
	delete(l.missed, node)
	var cleared []inventory.Observation
	for key, misses := range previous {
		if _, still := current[key]; still {
			continue
		}
		if optionalSource(key.source) && misses+1 < statsFailuresBeforeClear {
			current[key] = misses + 1
			continue
		}
		if l.release(key, node) {
			cleared = append(cleared, emptyReading(key, now))
		}
	}
	for key, misses := range current {
		if misses == 0 {
			l.owners[key] = node
		}
	}
	l.nodes[node] = current
	return cleared
}

// release forgets that node published key and reports whether node was
// the one that may clear it.
func (l *publishLog) release(key published, node string) bool {
	if owner, ok := l.owners[key]; ok && owner != node {
		return false
	}
	delete(l.owners, key)
	return true
}

// failed counts one failed poll of node. Once statsFailuresBeforeClear
// polls in a row failed, it returns empty observations for everything the
// node published and forgets it.
func (l *publishLog) failed(
	node string, now time.Time,
) []inventory.Observation {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.missed[node]++
	if l.missed[node] < statsFailuresBeforeClear {
		return nil
	}
	return l.forgetLocked(node, now)
}

// forgetNodes clears and forgets every node not in present: a node that
// left the cluster is never polled again, so nothing else would.
func (l *publishLog) forgetNodes(
	present map[string]bool, now time.Time,
) []inventory.Observation {
	l.mu.Lock()
	defer l.mu.Unlock()
	var cleared []inventory.Observation
	for node := range l.nodes {
		if !present[node] {
			cleared = append(cleared, l.forgetLocked(node, now)...)
		}
	}
	for node := range l.missed {
		if !present[node] {
			delete(l.missed, node)
		}
	}
	return cleared
}

// forgetLocked drops node and returns empty readings for what it still
// owned. The caller holds l.mu.
func (l *publishLog) forgetLocked(
	node string, now time.Time,
) []inventory.Observation {
	var cleared []inventory.Observation
	for key := range l.nodes[node] {
		if l.release(key, node) {
			cleared = append(cleared, emptyReading(key, now))
		}
	}
	delete(l.nodes, node)
	return cleared
}

// emptyReading replaces the source's attributes of an entity with none.
func emptyReading(key published, now time.Time) inventory.Observation {
	return inventory.Observation{
		Kind: inventory.Observed, Source: key.source, At: now,
		Entity: key.entity,
	}
}

// clearAfterFailure submits the empty readings a failed poll leads to.
func (p *StatsPoller) clearAfterFailure(
	ctx context.Context, node inventory.EntityID, now time.Time,
) {
	if cleared := p.published.failed(node.Name, now); len(cleared) > 0 {
		p.cfg.Submit(ctx, cleared...)
	}
}

// clearDeparted submits the empty readings of nodes that left the cluster.
func (p *StatsPoller) clearDeparted(
	ctx context.Context, nodes []inventory.EntityID, now time.Time,
) {
	present := make(map[string]bool, len(nodes))
	for _, node := range nodes {
		present[node.Name] = true
	}
	if cleared := p.published.forgetNodes(present, now); len(cleared) > 0 {
		p.cfg.Submit(ctx, cleared...)
	}
}
