package app

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
)

// The model self-health line carries the sizes of every structure that
// could grow, and the live heap that tells a leak from garbage.
func TestModelHealthFieldsNameTheBigStructures(t *testing.T) {
	model := inventory.NewModel(inventory.Options{})
	fields := modelHealthFields(model)
	got := map[string]bool{}
	for i := 0; i+1 < len(fields); i += 2 {
		got[fields[i].(string)] = true
	}
	for _, key := range []string{"heapLiveMiB", "heapGoalMiB", "goroutines",
		"entities", "tombstones", "notes", "noteParts", "recentChanges",
		"churnChanges", "baselines", "healthMarks"} {
		if !got[key] {
			t.Errorf("self-health line has no %q", key)
		}
	}
}

func TestModelHealthLogIsDueOncePerInterval(t *testing.T) {
	var l modelHealthLog
	at := time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC)
	model := inventory.NewModel(inventory.Options{})
	l.log(model, at)
	first := l.last
	l.log(model, at.Add(selfHealthEvery/2))
	if !l.last.Equal(first) {
		t.Fatal("logged again before the interval passed")
	}
	l.log(model, at.Add(selfHealthEvery))
	if l.last.Equal(first) {
		t.Fatal("did not log when the interval passed")
	}
}
