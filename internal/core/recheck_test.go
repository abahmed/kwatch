package core

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/knowledge"
)

func TestRecheckKeepsEarliest(t *testing.T) {
	r := newRechecks()
	id := knowledge.NewEntityID("pod", "default", "api")
	t1 := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	t2 := t1.Add(5 * time.Minute)

	r.schedule(id, t2)
	r.schedule(id, t1)

	next := r.next()
	if next != t1 {
		t.Errorf("next() = %v, want %v", next, t1)
	}
}

func TestRecheckDueOrderIsEarliest(t *testing.T) {
	r := newRechecks()
	id1 := knowledge.NewEntityID("pod", "default", "api-1")
	id2 := knowledge.NewEntityID("pod", "default", "api-2")

	t1 := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	t2 := t1.Add(1 * time.Minute)

	r.schedule(id2, t2)
	r.schedule(id1, t1)

	now := t1.Add(30 * time.Second)
	due := r.due(now)

	if len(due) != 1 || due[0] != id1 {
		t.Errorf("due() = %v, want [%v]", due, id1)
	}
}

func TestRecheckNextReturnsZeroWhenEmpty(t *testing.T) {
	r := newRechecks()
	next := r.next()
	if next != (time.Time{}) {
		t.Errorf("next() on empty = %v, want zero", next)
	}
}
