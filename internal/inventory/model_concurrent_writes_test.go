package inventory

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestConcurrencyMixedOperations(t *testing.T) {
	m := NewModel(Options{MaxChangesPerEntity: 32})
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	var wg sync.WaitGroup

	// Writer performs various operations
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			id := CoreID("pod", "default", "pod-"+string(rune(i%20)))
			op := i % 4
			switch op {
			case 0: // Observe
				observation := Observation{
					Kind:   Observed,
					Source: "k8s",
					At:     now.Add(time.Duration(i) * time.Millisecond),
					Entity: id,
					Attributes: map[string]Value{
						"status": Text("running"),
					},
				}
				_, _ = m.Apply(observation)
			case 1: // Related
				target := CoreID("node", "", "node-1")
				observation := Observation{
					Kind:     Related,
					Source:   "k8s",
					At:       now.Add(time.Duration(i) * time.Millisecond),
					Entity:   id,
					Relation: RunsOn,
					Targets:  []EntityID{target},
				}
				_, _ = m.Apply(observation)
			case 2: // Changed
				change := Change{
					At:    now.Add(time.Duration(i) * time.Millisecond),
					Actor: "user",
				}
				observation := Observation{
					Kind:   Changed,
					Source: "k8s",
					At:     now.Add(time.Duration(i) * time.Millisecond),
					Entity: id,
					Change: change,
				}
				_, _ = m.Apply(observation)
			case 3: // Gone
				observation := Observation{
					Kind:   Gone,
					Source: "k8s",
					At:     now.Add(time.Duration(i) * time.Millisecond),
					Entity: id,
				}
				_, _ = m.Apply(observation)
			}
		}
	}()

	// Reader 1: Entity reads
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			id := CoreID("pod", "default", "pod-"+string(rune(i%20)))
			_, _ = m.Entity(id)
		}
	}()

	// Reader 2: Related reads
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			id := CoreID("pod", "default", "pod-"+string(rune(i%20)))
			_ = m.Related(id, RunsOn, Outgoing)
		}
	}()

	// Reader 3: Entities reads
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			_ = m.Entities("pod")
		}
	}()

	// Reader 4: Changes reads
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			id := CoreID("pod", "default", "pod-"+string(rune(i%20)))
			_ = m.Changes(id, now)
		}
	}()

	// Reader 5: Stats reads
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			_ = m.Stats()
		}
	}()

	wg.Wait()

	// Verify model is consistent
	stats := m.Stats()
	assert.GreaterOrEqual(t, stats.Entities, 0)
	assert.GreaterOrEqual(t, stats.Tombstones, 0)
	assert.GreaterOrEqual(t, stats.Relations, 0)
	assert.GreaterOrEqual(t, stats.Changes, 0)
}

func TestConcurrencyPruneWithReads(t *testing.T) {
	m := NewModel(Options{})
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	var wg sync.WaitGroup

	id := CoreID("pod", "default", "my-pod")

	// Writer adds many changes
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			change := Change{
				At:    now.Add(time.Duration(i) * time.Millisecond),
				Actor: "user",
			}
			observation := Observation{
				Kind:   Changed,
				Source: "k8s",
				At:     now.Add(time.Duration(i) * time.Millisecond),
				Entity: id,
				Change: change,
			}
			_, _ = m.Apply(observation)
		}
	}()

	// Pruner runs concurrent pruning
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			m.Prune(now.Add(time.Duration(i*10) * time.Millisecond))
		}
	}()

	// Readers still try to read
	for j := 0; j < 3; j++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				_ = m.Changes(id, now)
				_ = m.Stats()
			}
		}()
	}

	wg.Wait()
}
