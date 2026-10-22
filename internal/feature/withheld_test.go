package feature

import (
	"strconv"
	"testing"
	"time"
)

var withheldStart = time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)

func TestWithheldCountsDistinctFindingsOnce(t *testing.T) {
	w := NewWithheld()
	w.Record(withheldStart, WithheldReadiness, "pod-a")
	w.Record(withheldStart.Add(time.Hour), WithheldReadiness, "pod-a")
	w.Record(withheldStart.Add(48*time.Hour), WithheldReadiness, "pod-a")
	w.Record(withheldStart, WithheldGates, "pod-a")
	w.Record(withheldStart, WithheldSharedCauses, "incident-1")

	got := w.Counts(withheldStart.Add(48 * time.Hour))
	want := WithheldCounts{1, 1, 0, 1}
	if got != want {
		t.Fatalf("counts = %v, want %v", got, want)
	}
	if text := got.Text(); text != "1 readiness probe, 1 shared cause, "+
		"1 scheduling gate" {
		t.Errorf("text = %q", text)
	}
}

func TestWithheldForgetsDaysOutsideTheWindow(t *testing.T) {
	w := NewWithheld()
	w.Record(withheldStart, WithheldPreemption, "pod-a")
	later := withheldStart.Add(WithheldDays * 24 * time.Hour)
	if got := w.Counts(later).Total(); got != 0 {
		t.Fatalf("total after the window = %d, want 0", got)
	}
	w.Record(later, WithheldPreemption, "pod-a")
	if got := w.Counts(later).Total(); got != 1 {
		t.Fatalf("total = %d, want the finding counted again", got)
	}
}

func TestWithheldMemoryIsBounded(t *testing.T) {
	w := NewWithheld()
	for i := 0; i < withheldPerDay+100; i++ {
		w.Record(withheldStart, WithheldReadiness, strconv.Itoa(i))
	}
	if got := w.Counts(withheldStart).Total(); got != withheldPerDay {
		t.Fatalf("total = %d, want the cap %d", got, withheldPerDay)
	}
}

func TestWithheldNilCountsNothing(t *testing.T) {
	var w *Withheld
	w.Record(withheldStart, WithheldReadiness, "pod-a")
	if got := w.Counts(withheldStart).Total(); got != 0 {
		t.Fatalf("nil total = %d", got)
	}
	if text := (WithheldCounts{}).Text(); text != "" {
		t.Errorf("zero text = %q", text)
	}
}
