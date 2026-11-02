package incident

import (
	"fmt"
	"testing"
)

func TestStoreDetailKeepsTheStartOfTheTrouble(t *testing.T) {
	r := newRig(t, Config{})
	announced(t, r, podSig("web"))
	id := r.only().ID
	first := "panic: assignment to entry in nil map"
	r.m.StoreDetail(id, Detail{Facts: []string{first}})

	for update := range 6 {
		var facts []string
		for i := range 5 {
			facts = append(facts, fmt.Sprintf("retry %d.%d", update, i))
		}
		r.m.StoreDetail(id, Detail{Facts: facts})
	}

	got := r.m.Incident(id).Facts
	if len(got) != maxFacts || got[0] != first ||
		got[8] != "… 7 lines skipped …" || got[len(got)-1] != "retry 5.4" {
		t.Fatalf("kept %d facts: %q", len(got), got)
	}
}
