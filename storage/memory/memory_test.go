package memory

import (
	"sync"
	"testing"

	storage "github.com/abahmed/kwatch/storage"
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

	mem.AddPodContainer("test", "test")
	mem.AddPodContainer("test", "test2")

	if v, ok := mem.smap.Load("test"); !ok {
		t.Errorf("expected to find value in pod test")
	} else {
		containers := v.(map[string]bool)
		if _, ok = containers["test"]; !ok {
			t.Errorf("expected to find container test in pod test")
		}

		if _, ok = containers["test2"]; !ok {
			t.Errorf("expected to find container test2 in pod test")
		}
	}
}

func TestHasPodContainer(t *testing.T) {
	mem := &memory{
		smap: sync.Map{},
	}

	mem.AddPodContainer("test", "test")
	mem.AddPodContainer("test", "test2")

	mem.DelPodContainer("test", "test")
	mem.DelPodContainer("test3", "test")

	if !mem.HasPodContainer("test", "test2") {
		t.Errorf("expected to find container test2 in pod test")
	}

	if mem.HasPodContainer("test", "test") {
		t.Errorf("expected not to find container test in pod test")
	}

	if mem.HasPodContainer("test", "test6") {
		t.Errorf("expected not to find container test6 in pod test")
	}

	if mem.HasPodContainer("test4", "test") {
		t.Errorf("expected to not find container test in pod test4")
	}
}

func TestDelPodContainer(t *testing.T) {
	mem := &memory{
		smap: sync.Map{},
	}

	mem.AddPodContainer("test", "test")
	mem.AddPodContainer("test", "test2")

	mem.DelPodContainer("test", "test")
	mem.DelPodContainer("test3", "test")

	if v, ok := mem.smap.Load("test"); !ok {
		t.Errorf("expected to find value in pod test")
	} else {
		containers := v.(map[string]bool)
		if _, ok = containers["test"]; ok {
			t.Errorf("expected not to find container test in pod test")
		}
	}
}

func TestDelPod(t *testing.T) {
	mem := &memory{
		smap: sync.Map{},
	}

	mem.AddPodContainer("test", "test")
	mem.AddPodContainer("test", "test2")

	mem.DelPod("test")
	mem.DelPod("test3")

	if _, ok := mem.smap.Load("test"); ok {
		t.Errorf("expected not to find pod test")
	}
}
