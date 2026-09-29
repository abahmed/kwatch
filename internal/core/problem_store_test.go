package core

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/problem"
)

func TestProblemStoreRoundTripsProblems(t *testing.T) {
	ps := NewProblemStore(openTempStore(t))
	root := knowledge.NewEntityID("node", "", "n1")
	records := []problem.Record{
		{ID: "a", Root: root, Opened: time.Unix(10, 0).UTC()},
		{ID: "b", Root: root},
	}

	if err := ps.SaveProblems(records); err != nil {
		t.Fatal(err)
	}
	got, err := ps.LoadProblems()

	if err != nil || len(got) != 2 {
		t.Fatalf("got %v, %v", got, err)
	}
}

func TestProblemStoreSaveRemovesDroppedProblems(t *testing.T) {
	ps := NewProblemStore(openTempStore(t))
	_ = ps.SaveProblems([]problem.Record{{ID: "a"}, {ID: "b"}})

	if err := ps.SaveProblems([]problem.Record{{ID: "b"}}); err != nil {
		t.Fatal(err)
	}
	got, _ := ps.LoadProblems()

	if len(got) != 1 || got[0].ID != "b" {
		t.Fatalf("got %v, want only b", got)
	}
}

func TestProblemStoreRoundTripsFingerprints(t *testing.T) {
	ps := NewProblemStore(openTempStore(t))

	err := ps.SaveFingerprints(map[string]any{"k": "v", "j": "w"})
	got, loadErr := ps.LoadFingerprints()

	if err != nil || loadErr != nil {
		t.Fatal(err, loadErr)
	}
	if len(got) != 2 || got["k"] != "v" {
		t.Fatalf("got %v", got)
	}
}

func TestProblemStoreLoadFailsAfterClose(t *testing.T) {
	s := openTempStore(t)
	ps := NewProblemStore(s)
	_ = s.Close()

	if _, err := ps.LoadProblems(); err == nil {
		t.Error("LoadProblems on a closed store must fail")
	}
	if _, err := ps.LoadFingerprints(); err == nil {
		t.Error("LoadFingerprints on a closed store must fail")
	}
	if err := ps.SaveProblems(nil); err == nil {
		t.Error("SaveProblems on a closed store must fail")
	}
}
