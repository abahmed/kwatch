package flight

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/replay"
)

// record writes n observations of pod, one per minute from the clock.
func record(rec *Recorder, clock *clockAt, pod string, n int) {
	for i := 0; i < n; i++ {
		submitAll(rec, podObserved(pod, float64(i)))
		clock.advance(time.Minute)
	}
}

func TestExportWindowIsALoadableLog(t *testing.T) {
	clock := newClock()
	rec, ring, _ := newRecorder(t, clock)
	record(rec, clock, "old", 1)   // before the window; never changes
	record(rec, clock, "busy", 10) // inside the window
	rec.Flush()
	since := base.Add(2 * time.Minute)

	var out bytes.Buffer
	sum, err := ExportFrom(ring, since, time.Time{}, &out)
	if err != nil {
		t.Fatal(err)
	}
	log, err := replay.Read(&out)
	if err != nil {
		t.Fatalf("the export is not a loadable log: %v", err)
	}
	if !log.Start.Equal(since) || sum.Entries != len(log.Entries) {
		t.Errorf("start %s, summary %+v, entries %d", log.Start, sum,
			len(log.Entries))
	}
	if sum.Baseline != 2 {
		t.Errorf("baseline = %d, want both pods seen before the window",
			sum.Baseline)
	}
	if !log.Entries[0].At.Equal(since) {
		t.Errorf("the baseline must start the log at %s, got %s", since,
			log.Entries[0].At)
	}
}

func TestExportBaselineForgetsGoneEntities(t *testing.T) {
	clock := newClock()
	rec, ring, _ := newRecorder(t, clock)
	submitAll(rec, podObserved("gone", 1))
	clock.advance(time.Minute)
	submitAll(rec, inventory.Observation{Kind: inventory.Gone,
		Source: "kubernetes", Entity: inventory.CoreID("pod", "shop", "gone")})
	clock.advance(time.Minute)
	record(rec, clock, "live", 3)
	rec.Flush()
	var out bytes.Buffer
	sum, err := ExportFrom(ring, base.Add(3*time.Minute), time.Time{}, &out)
	if err != nil || sum.Baseline != 1 {
		t.Errorf("baseline = %d, err = %v; only the live pod, not the "+
			"deleted one",
			sum.Baseline, err)
	}
}

func TestExportWithoutSinceStartsAtTheFirstEntry(t *testing.T) {
	clock := newClock()
	rec, ring, _ := newRecorder(t, clock)
	clock.advance(time.Hour)
	record(rec, clock, "a", 2)
	rec.Flush()
	var out bytes.Buffer
	if _, err := ExportFrom(ring, time.Time{}, time.Time{}, &out); err != nil {
		t.Fatal(err)
	}
	log, err := replay.Read(&out)
	if err != nil || !log.Start.Equal(base.Add(time.Hour)) {
		t.Errorf("start = %s, err = %v", log.Start, err)
	}
}

func TestExportEmptyWindowSaysSo(t *testing.T) {
	_, ring, _ := newRecorder(t, newClock())
	var out bytes.Buffer
	_, err := ExportFrom(ring, base, time.Time{}, &out)
	if !errors.Is(err, ErrNoRecording) {
		t.Errorf("err = %v, want ErrNoRecording", err)
	}
}

func TestExportReadsACopyOfTheStateFile(t *testing.T) {
	clock := newClock()
	dir := t.TempDir()
	path := filepath.Join(dir, "state.db")
	store := openStore(t, path, clock.Now) // stays open: kwatch is running
	salt, _ := LoadSalt(store)
	rec, _ := NewRecorder(NewRing(store), salt, clock.Now)
	record(rec, clock, "api", 5)
	rec.Flush()

	out := filepath.Join(dir, "incident.jsonl")
	sum, err := Export(ExportOptions{StatePath: path, Out: out})
	if err != nil {
		t.Fatal(err)
	}
	if sum.Entries != 5 {
		t.Errorf("entries = %d, want 5", sum.Entries)
	}
	f, err := os.Open(out)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if _, err := replay.Read(f); err != nil {
		t.Error(err)
	}
	leftovers, _ := filepath.Glob(filepath.Join(dir, "*.copy*"))
	if len(leftovers) != 0 {
		t.Errorf("temporary copy left behind: %v", leftovers)
	}
}

func TestExportMissingStateFileIsAnError(t *testing.T) {
	_, err := Export(ExportOptions{
		StatePath: filepath.Join(t.TempDir(), "none.db"),
		Out:       filepath.Join(t.TempDir(), "o.jsonl")})
	if err == nil {
		t.Error("a missing state file must fail")
	}
}

func TestParseWindow(t *testing.T) {
	now := base
	since, until, err := ParseWindow("2h", "30m", now)
	if err != nil || !since.Equal(now.Add(-2*time.Hour)) ||
		!until.Equal(now.Add(-30*time.Minute)) {
		t.Errorf("relative: %s %s %v", since, until, err)
	}
	abs := "2026-10-13T08:00:00Z"
	since, _, err = ParseWindow(abs, "", now)
	if err != nil || since.Format(time.RFC3339) != abs {
		t.Errorf("absolute: %s %v", since, err)
	}
	if _, _, err := ParseWindow("soon", "", now); err == nil {
		t.Error("garbage accepted")
	}
	if _, _, err := ParseWindow("1h", "2h", now); err == nil {
		t.Error("until before since accepted")
	}
	_ = fmt.Sprint()
}
