package incident

import (
	"sync"
	"testing"
)

func TestManagerIncidentsAreDetachedAndSorted(t *testing.T) {
	r := newRig(t, Config{})
	r.raise(at(0), podSig("web"))
	r.raise(at(0), podSig("api"))

	got := r.m.Incidents()
	if len(got) != 2 || got[0].ID > got[1].ID {
		t.Fatalf("want two incidents ordered by ID, got %+v", got)
	}
	got[0].Members = nil
	got[0].Impact = append(got[0].Impact, got[0].Root)
	got[0].State = Resolved

	again := r.m.Incidents()
	if len(again[0].Members) == 0 || again[0].State == Resolved {
		t.Fatalf("snapshot aliases manager state: %+v", again[0])
	}
}

func TestManagerIncidentsAreSafeDuringApply(t *testing.T) {
	r := newRig(t, Config{})
	var wg sync.WaitGroup
	wg.Add(1)
	done := make(chan struct{})
	go func() {
		defer wg.Done()
		for {
			select {
			case <-done:
				return
			default:
				_ = r.m.Incidents()
			}
		}
	}()
	for i := 0; i < 50; i++ {
		r.raise(at(0), podSig("web"))
		r.tick(at(0))
	}
	close(done)
	wg.Wait()
}
