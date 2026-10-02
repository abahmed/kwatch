package inventory

import (
	"sync"
	"testing"
	"time"
)

func TestConcurrencyObserveAndRead(t *testing.T) {
	m := NewModel(Options{})
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	var wg sync.WaitGroup

	// Writer goroutine applies 1000 observations
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			id := CoreID("pod", "default", "pod-"+string(rune(i%10)))
			observation := Observation{
				Kind:   Observed,
				Source: "k8s",
				At:     now.Add(time.Duration(i) * time.Millisecond),
				Entity: id,
				Attributes: map[string]Value{
					"index": Number(float64(i)),
				},
			}
			_, _ = m.Apply(observation)
		}
	}()

	// Reader goroutines read Entity
	for j := 0; j < 5; j++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				id := CoreID("pod", "default", "pod-"+string(rune(i%10)))
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

	podID := CoreID("pod", "default", "my-pod")
	pvcID := CoreID("pvc", "default", "my-pvc")

	// Setup relations
	observation := Observation{
		Kind:     Related,
		Source:   "k8s",
		At:       now,
		Entity:   podID,
		Relation: Mounts,
		Targets:  []EntityID{pvcID},
	}
	_, _ = m.Apply(observation)

	// Writer updates relations
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			if i%2 == 0 {
				observation := Observation{
					Kind:     Related,
					Source:   "k8s",
					At:       now.Add(time.Duration(i) * time.Millisecond),
					Entity:   podID,
					Relation: Mounts,
					Targets:  []EntityID{pvcID},
				}
				_, _ = m.Apply(observation)
			} else {
				observation := Observation{
					Kind:     Related,
					Source:   "k8s",
					At:       now.Add(time.Duration(i) * time.Millisecond),
					Entity:   podID,
					Relation: Mounts,
					Targets:  []EntityID{},
				}
				_, _ = m.Apply(observation)
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
			id := CoreID("pod", "default", "pod-"+string(rune(i)))
			observation := Observation{
				Kind:   Observed,
				Source: "k8s",
				At:     now.Add(time.Duration(i) * time.Millisecond),
				Entity: id,
			}
			_, _ = m.Apply(observation)
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

	id := CoreID("pod", "default", "my-pod")

	// Writer adds changes
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 300; i++ {
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
			id := CoreID("pod", "default", "pod-"+string(rune(i%50)))
			observation := Observation{
				Kind:   Observed,
				Source: "k8s",
				At:     now.Add(time.Duration(i) * time.Millisecond),
				Entity: id,
			}
			_, _ = m.Apply(observation)

			if i > 0 && i%10 == 0 {
				target := CoreID("node", "", "node-"+string(rune(i%5)))
				observation := Observation{
					Kind:     Related,
					Source:   "k8s",
					At:       now.Add(time.Duration(i) * time.Millisecond),
					Entity:   id,
					Relation: RunsOn,
					Targets:  []EntityID{target},
				}
				_, _ = m.Apply(observation)
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

func TestConcurrencyExistsRead(t *testing.T) {
	m := NewModel(Options{})
	now := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)
	var wg sync.WaitGroup

	id := CoreID("pod", "default", "my-pod")

	// Writer observes and removes entity repeatedly
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			if i%2 == 0 {
				observation := Observation{
					Kind:   Observed,
					Source: "k8s",
					At:     now.Add(time.Duration(i) * time.Millisecond),
					Entity: id,
				}
				_, _ = m.Apply(observation)
			} else {
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

	podID := CoreID("pod", "default", "my-pod")
	nodeID := CoreID("node", "", "node-1")
	pvcID := CoreID("pvc", "default", "my-pvc")
	cmID := CoreID("configmap", "default", "config")

	// Setup initial relations
	observations := []Observation{
		{Kind: Related, Source: "k8s", At: now, Entity: podID,
			Relation: RunsOn, Targets: []EntityID{nodeID}},
		{Kind: Related, Source: "k8s", At: now, Entity: podID,
			Relation: Mounts, Targets: []EntityID{pvcID}},
		{Kind: Related, Source: "k8s", At: now, Entity: podID,
			Relation: References, Targets: []EntityID{cmID}},
	}
	for _, f := range observations {
		_, _ = m.Apply(f)
	}

	// Writer updates relations
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 100; i++ {
			target := CoreID("node", "", "node-"+string(rune(i%3)))
			observation := Observation{
				Kind:     Related,
				Source:   "k8s",
				At:       now.Add(time.Duration(i) * time.Millisecond),
				Entity:   podID,
				Relation: RunsOn,
				Targets:  []EntityID{target},
			}
			_, _ = m.Apply(observation)
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
