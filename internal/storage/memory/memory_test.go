package memory

import (
	"fmt"
	"sync"
	"testing"

	storage "github.com/abahmed/kwatch/internal/storage"
)

func TestMemory(t *testing.T) {
	m := NewMemory()
	_, ok := m.(storage.Storage)
	if !ok {
		t.Errorf("expected to return Storage interface")
	}
}

func TestAddPodContainer(t *testing.T) {
	mem := &memory{
		smap: sync.Map{},
	}

	mem.AddPodContainer("default", "test", "container1", &storage.ContainerState{})
	mem.AddPodContainer("default", "test", "container2", &storage.ContainerState{})

	if v, ok := mem.smap.Load(mem.getKey("default", "test")); !ok {
		t.Errorf("expected to find value in pod test")
	} else {
		containers := v.(map[string]*storage.ContainerState)
		if _, ok = containers["container1"]; !ok {
			t.Errorf("expected to find container container1 in pod test")
		}

		if _, ok = containers["container2"]; !ok {
			t.Errorf("expected to find container container2 in pod test")
		}
	}
}

func TestHasPodContainer(t *testing.T) {
	mem := &memory{
		smap: sync.Map{},
	}

	mem.AddPodContainer("default", "test", "test", &storage.ContainerState{})
	mem.AddPodContainer("default", "test", "test2", &storage.ContainerState{})

	mem.DelPodContainer("default", "test", "test")
	mem.DelPodContainer("default", "test3", "test")

	if !mem.HasPodContainer("default", "test", "test2") {
		t.Errorf("expected to find container test2 in pod test")
	}

	if mem.HasPodContainer("default", "test", "test") {
		t.Errorf("expected not to find container test in pod test")
	}

	if mem.HasPodContainer("default", "test", "test6") {
		t.Errorf("expected not to find container test6 in pod test")
	}

	if mem.HasPodContainer("default", "test4", "test") {
		t.Errorf("expected to not find container test in pod test4")
	}
}

func TestDelPodContainer(t *testing.T) {
	mem := &memory{
		smap: sync.Map{},
	}

	mem.AddPodContainer("default", "test", "test", &storage.ContainerState{})
	mem.AddPodContainer("default", "test", "test2", &storage.ContainerState{})

	mem.DelPodContainer("default", "test", "test")
	mem.DelPodContainer("default", "test3", "test")

	if v, ok := mem.smap.Load(mem.getKey("default", "test")); !ok {
		t.Errorf("expected to find value in pod test")
	} else {
		containers := v.(map[string]*storage.ContainerState)
		if _, ok = containers["test"]; ok {
			t.Errorf("expected not to find container test in pod test")
		}
	}
}

func TestGetPodContainer(t *testing.T) {
	mem := &memory{
		smap: sync.Map{},
	}

	mem.AddPodContainer("default", "test", "test1", &storage.ContainerState{})
	mem.AddPodContainer("default", "test", "test2", &storage.ContainerState{})

	state := mem.GetPodContainer("default", "test", "test1")
	if state == nil {
		t.Errorf("expected to find value in pod test")
	}

	state2 := mem.GetPodContainer("default", "test", "test3")
	if state2 != nil {
		t.Errorf("expected to be nil as container does not exist")
	}

	state3 := mem.GetPodContainer("default", "test3", "test1")
	if state3 != nil {
		t.Errorf("expected to be nil as pod does not exist")
	}
}

func TestMemoryConcurrentAccess(t *testing.T) {
	mem := &memory{
		smap: sync.Map{},
	}

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			name := fmt.Sprintf("container-%d", i)
			mem.AddPodContainer("default", "shared-pod", name, &storage.ContainerState{RestartCount: int32(i)})
		}(i)
	}
	wg.Wait()

	if v, ok := mem.smap.Load(mem.getKey("default", "shared-pod")); !ok {
		t.Errorf("expected to find pod shared-pod")
	} else {
		containers := v.(map[string]*storage.ContainerState)
		if len(containers) != 50 {
			t.Errorf("expected 50 containers, got %d", len(containers))
		}
	}

	var wg2 sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg2.Add(1)
		go func(i int) {
			defer wg2.Done()
			name := fmt.Sprintf("container-%d", i)
			mem.HasPodContainer("default", "shared-pod", name)
			mem.GetPodContainer("default", "shared-pod", name)
		}(i)
	}
	wg2.Wait()

	var wg3 sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg3.Add(1)
		go func(i int) {
			defer wg3.Done()
			name := fmt.Sprintf("container-%d", i)
			mem.DelPodContainer("default", "shared-pod", name)
		}(i)
	}
	wg3.Wait()

	if v, ok := mem.smap.Load(mem.getKey("default", "shared-pod")); ok {
		containers := v.(map[string]*storage.ContainerState)
		if len(containers) != 0 {
			t.Errorf("expected 0 containers after deletion, got %d", len(containers))
		}
	}
}

func TestDelPod(t *testing.T) {
	mem := &memory{
		smap: sync.Map{},
	}

	mem.AddPodContainer("default", "test", "test1", &storage.ContainerState{})
	mem.AddPodContainer("default", "test", "test2", &storage.ContainerState{})

	mem.DelPod("default", "test")
	mem.DelPod("default", "test3")

	if _, ok := mem.smap.Load(mem.getKey("default", "test")); ok {
		t.Errorf("expected not to find pod test")
	}
}

func TestAddNode(t *testing.T) {
	mem := &memory{
		nmap: sync.Map{},
	}

	mem.AddNode("default-node-1")
	mem.AddNode("default-node-2")

	if _, ok := mem.nmap.Load("default-node-1"); !ok {
		t.Errorf("expected to find node default-node-1")
	}
}

func TestHasNode(t *testing.T) {
	mem := &memory{
		nmap: sync.Map{},
	}

	mem.AddNode("default-node-1")
	mem.AddNode("default-node-2")

	if !mem.HasNode(("default-node-1")) {
		t.Errorf("expected to find node default-node-1")
	}

	if mem.HasNode("default-node-3") {
		t.Errorf("expected not to find node default-node-3")
	}
}

func TestDelNode(t *testing.T) {
	mem := &memory{
		smap: sync.Map{},
	}

	mem.AddNode("default-node-1")
	mem.AddNode("default-node-2")

	mem.DelNode("default-node-1")
	mem.DelNode("default-node-2")

	if _, ok := mem.nmap.Load("default-node-1"); ok {
		t.Errorf("expected not to find node default-node-1")
	}
}
