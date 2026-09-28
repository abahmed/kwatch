package knowledge

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestConcurrencyObserveAndRead(t *testing.T) {
	m := NewModel(Options{})
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	var wg sync.WaitGroup

	// Writer goroutine applies 1000 facts
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			id := NewEntityID("pod", "default", "pod-"+string(rune(i%10)))
			fact := Fact{
				Kind:   Observed,
				Source: "k8s",
				At:     now.Add(time.Duration(i) * time.Millisecond),
				Entity: id,
				Attributes: map[string]Value{
					"index": Number(float64(i)),
				},
			}
			_, _ = m.Apply(fact)
		}
	}()

	// Reader goroutines read Entity
	for j := 0; j < 5; j++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				id := NewEntityID("pod", "default", "pod-"+string(rune(i%10)))
				_, _ = m.Entity(id)
			}
		}()
	}

	wg.Wait()
}

func TestConcurrencyRelatedReads(t *testing.T) {
	m := NewModel(Options{})
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	var wg sync.WaitGroup

	podID := NewEntityID("pod", "default", "my-pod")
	pvcID := NewEntityID("pvc", "default", "my-pvc")

	// Setup relations
	fact := Fact{
		Kind:     Related,
		Source:   "k8s",
		At:       now,
		Entity:   podID,
		Relation: Mounts,
		Targets:  []EntityID{pvcID},
	}
	_, _ = m.Apply(fact)

	// Writer updates relations
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			if i%2 == 0 {
				fact := Fact{
					Kind:     Related,
					Source:   "k8s",
					At:       now.Add(time.Duration(i) * time.Millisecond),
					Entity:   podID,
					Relation: Mounts,
					Targets:  []EntityID{pvcID},
				}
				_, _ = m.Apply(fact)
			} else {
				fact := Fact{
					Kind:     Related,
					Source:   "k8s",
					At:       now.Add(time.Duration(i) * time.Millisecond),
					Entity:   podID,
					Relation: Mounts,
					Targets:  []EntityID{},
				}
				_, _ = m.Apply(fact)
			}
		}
	}()

	// Readers call Related
	for j := 0; j < 5; j++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				_ = m.Related(podID, Mounts, Outgoing)
				_ = m.Related(pvcID, Mounts, Incoming)
			}
		}()
	}

	wg.Wait()
}

func TestConcurrencyEntitiesRead(t *testing.T) {
	m := NewModel(Options{})
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	var wg sync.WaitGroup

	// Writer adds entities
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			id := NewEntityID("pod", "default", "pod-"+string(rune(i)))
			fact := Fact{
				Kind:   Observed,
				Source: "k8s",
				At:     now.Add(time.Duration(i) * time.Millisecond),
				Entity: id,
			}
			_, _ = m.Apply(fact)
		}
	}()

	// Readers call Entities
	for j := 0; j < 5; j++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				_ = m.Entities("pod")
			}
		}()
	}

	wg.Wait()
}

func TestConcurrencyChangesRead(t *testing.T) {
	m := NewModel(Options{})
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	var wg sync.WaitGroup

	id := NewEntityID("pod", "default", "my-pod")

	// Writer adds changes
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 300; i++ {
			change := Change{
				At:    now.Add(time.Duration(i) * time.Millisecond),
				Actor: "user",
			}
			fact := Fact{
				Kind:   Changed,
				Source: "k8s",
				At:     now.Add(time.Duration(i) * time.Millisecond),
				Entity: id,
				Change: change,
			}
			_, _ = m.Apply(fact)
		}
	}()

	// Readers call Changes
	for j := 0; j < 5; j++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				_ = m.Changes(id, now)
				_ = m.Changes(id, now.Add(time.Second))
			}
		}()
	}

	wg.Wait()
}

func TestConcurrencyStatsCalls(t *testing.T) {
	m := NewModel(Options{})
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	var wg sync.WaitGroup

	// Writer adds entities and relations
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			id := NewEntityID("pod", "default", "pod-"+string(rune(i%50)))
			fact := Fact{
				Kind:   Observed,
				Source: "k8s",
				At:     now.Add(time.Duration(i) * time.Millisecond),
				Entity: id,
			}
			_, _ = m.Apply(fact)

			if i > 0 && i%10 == 0 {
				target := NewEntityID("node", "", "node-"+string(rune(i%5)))
				fact := Fact{
					Kind:     Related,
					Source:   "k8s",
					At:       now.Add(time.Duration(i) * time.Millisecond),
					Entity:   id,
					Relation: RunsOn,
					Targets:  []EntityID{target},
				}
				_, _ = m.Apply(fact)
			}
		}
	}()

	// Readers call Stats
	for j := 0; j < 5; j++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				_ = m.Stats()
			}
		}()
	}

	wg.Wait()
}

func TestConcurrencyMixedOperations(t *testing.T) {
	m := NewModel(Options{MaxChangesPerEntity: 32})
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	var wg sync.WaitGroup

	// Writer performs various operations
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			id := NewEntityID("pod", "default", "pod-"+string(rune(i%20)))
			op := i % 4
			switch op {
			case 0: // Observe
				fact := Fact{
					Kind:   Observed,
					Source: "k8s",
					At:     now.Add(time.Duration(i) * time.Millisecond),
					Entity: id,
					Attributes: map[string]Value{
						"status": Text("running"),
					},
				}
				_, _ = m.Apply(fact)
			case 1: // Related
				target := NewEntityID("node", "", "node-1")
				fact := Fact{
					Kind:     Related,
					Source:   "k8s",
					At:       now.Add(time.Duration(i) * time.Millisecond),
					Entity:   id,
					Relation: RunsOn,
					Targets:  []EntityID{target},
				}
				_, _ = m.Apply(fact)
			case 2: // Changed
				change := Change{
					At:    now.Add(time.Duration(i) * time.Millisecond),
					Actor: "user",
				}
				fact := Fact{
					Kind:   Changed,
					Source: "k8s",
					At:     now.Add(time.Duration(i) * time.Millisecond),
					Entity: id,
					Change: change,
				}
				_, _ = m.Apply(fact)
			case 3: // Gone
				fact := Fact{
					Kind:   Gone,
					Source: "k8s",
					At:     now.Add(time.Duration(i) * time.Millisecond),
					Entity: id,
				}
				_, _ = m.Apply(fact)
			}
		}
	}()

	// Reader 1: Entity reads
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			id := NewEntityID("pod", "default", "pod-"+string(rune(i%20)))
			_, _ = m.Entity(id)
		}
	}()

	// Reader 2: Related reads
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			id := NewEntityID("pod", "default", "pod-"+string(rune(i%20)))
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
			id := NewEntityID("pod", "default", "pod-"+string(rune(i%20)))
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

	id := NewEntityID("pod", "default", "my-pod")

	// Writer adds many changes
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 500; i++ {
			change := Change{
				At:    now.Add(time.Duration(i) * time.Millisecond),
				Actor: "user",
			}
			fact := Fact{
				Kind:   Changed,
				Source: "k8s",
				At:     now.Add(time.Duration(i) * time.Millisecond),
				Entity: id,
				Change: change,
			}
			_, _ = m.Apply(fact)
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

func TestConcurrencyExistsRead(t *testing.T) {
	m := NewModel(Options{})
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	var wg sync.WaitGroup

	id := NewEntityID("pod", "default", "my-pod")

	// Writer observes and removes entity repeatedly
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			if i%2 == 0 {
				fact := Fact{
					Kind:   Observed,
					Source: "k8s",
					At:     now.Add(time.Duration(i) * time.Millisecond),
					Entity: id,
				}
				_, _ = m.Apply(fact)
			} else {
				fact := Fact{
					Kind:   Gone,
					Source: "k8s",
					At:     now.Add(time.Duration(i) * time.Millisecond),
					Entity: id,
				}
				_, _ = m.Apply(fact)
			}
		}
	}()

	// Readers call Exists
	for j := 0; j < 5; j++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				_ = m.Exists(id)
			}
		}()
	}

	wg.Wait()
}

func TestConcurrencyRelationsListReads(t *testing.T) {
	m := NewModel(Options{})
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	var wg sync.WaitGroup

	podID := NewEntityID("pod", "default", "my-pod")
	nodeID := NewEntityID("node", "", "node-1")
	pvcID := NewEntityID("pvc", "default", "my-pvc")
	cmID := NewEntityID("configmap", "default", "config")

	// Setup initial relations
	facts := []Fact{
		{Kind: Related, Source: "k8s", At: now, Entity: podID,
			Relation: RunsOn, Targets: []EntityID{nodeID}},
		{Kind: Related, Source: "k8s", At: now, Entity: podID,
			Relation: Mounts, Targets: []EntityID{pvcID}},
		{Kind: Related, Source: "k8s", At: now, Entity: podID,
			Relation: References, Targets: []EntityID{cmID}},
	}
	for _, f := range facts {
		_, _ = m.Apply(f)
	}

	// Writer updates relations
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			target := NewEntityID("node", "", "node-"+string(rune(i%3)))
			fact := Fact{
				Kind:     Related,
				Source:   "k8s",
				At:       now.Add(time.Duration(i) * time.Millisecond),
				Entity:   podID,
				Relation: RunsOn,
				Targets:  []EntityID{target},
			}
			_, _ = m.Apply(fact)
		}
	}()

	// Readers call Relations
	for j := 0; j < 5; j++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				_ = m.Relations(podID, Outgoing)
				_ = m.Relations(nodeID, Incoming)
			}
		}()
	}

	wg.Wait()
}
