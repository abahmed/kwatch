package metrics

import "testing"

func TestIncPanicCountsBackgroundWorkersApart(t *testing.T) {
	r := &Registry{}

	r.IncPanic("announce")
	r.IncPanic("storage-writer") // any name outside the decision path
	r.IncPanic("history-writer")

	if got := r.PanicCount("announce"); got != 1 {
		t.Errorf("announce = %d, want only its own panic", got)
	}
	if got := r.PanicCount("background"); got != 2 {
		t.Errorf("background = %d, want the two worker panics", got)
	}
}
