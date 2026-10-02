package replay_test

import (
	"bytes"
	"context"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/replay"
)

var errWrite = errors.New("write failed")

func fixedNow() time.Time { return rolloutStart }

func TestRecorderWritesAndForwards(t *testing.T) {
	now := time.Date(2026, 9, 29, 16, 0, 0, 0, time.FixedZone("x", 7200))
	var buf bytes.Buffer
	var forwarded []inventory.Observation
	rec, err := replay.NewRecorder(&buf, func() time.Time { return now },
		func(_ context.Context, obs ...inventory.Observation) {
			forwarded = append(forwarded, obs...)
		})
	if err != nil {
		t.Fatal(err)
	}
	observations := sampleLog().Entries
	rec.Submit(context.Background(), observations[0].Observation)
	now = now.Add(time.Second)
	rec.Submit(context.Background(), observations[1].Observation)
	if err := rec.Err(); err != nil {
		t.Fatal(err)
	}
	if len(forwarded) != 2 {
		t.Fatalf("forwarded %d observations", len(forwarded))
	}
	log, err := replay.Read(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if len(log.Entries) != 2 ||
		!log.Entries[1].At.Equal(now) ||
		log.Entries[1].At.Location() != time.UTC {
		t.Fatalf("recorded %+v", log.Entries)
	}
}

func TestRecorderKeepsForwardingAfterWriteError(t *testing.T) {
	writer := &failingWriter{after: 1}
	forwarded := 0
	rec, err := replay.NewRecorder(writer, fixedNow,
		func(_ context.Context, obs ...inventory.Observation) {
			forwarded += len(obs)
		})
	if err != nil {
		t.Fatal(err)
	}
	obs := sampleLog().Entries[0].Observation
	rec.Submit(context.Background(), obs, obs)
	rec.Submit(context.Background(), obs)
	if !errors.Is(rec.Err(), errWrite) {
		t.Fatalf("err = %v", rec.Err())
	}
	if forwarded != 3 {
		t.Fatalf("forwarded %d observations, want 3", forwarded)
	}
}

func TestRecorderRejectsIncompleteSetup(t *testing.T) {
	if _, err := replay.NewRecorder(nil, fixedNow, nil); err == nil {
		t.Fatal("expected an error without a writer")
	}
	if _, err := replay.NewRecorder(&bytes.Buffer{}, nil, nil); err == nil {
		t.Fatal("expected an error without a clock")
	}
	if _, err := replay.NewRecorder(&failingWriter{}, fixedNow,
		nil); err == nil {
		t.Fatal("expected the header write error")
	}
}

// Concurrent sources reach the engine in the order the log records
// them, so replaying the log replays what the engine saw.
func TestRecorderForwardsInRecordedOrder(t *testing.T) {
	var buf bytes.Buffer
	var forwarded []string
	rec, err := replay.NewRecorder(&buf, fixedNow,
		func(_ context.Context, obs ...inventory.Observation) {
			forwarded = append(forwarded, obs[0].Entity.Name)
		})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			rec.Submit(context.Background(), inventory.Observation{
				Kind: inventory.Observed, Source: "test",
				Entity: inventory.CoreID("pod", "ns", strconv.Itoa(i)),
			})
		}(i)
	}
	wg.Wait()
	log, err := replay.Read(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if len(log.Entries) != len(forwarded) {
		t.Fatalf("recorded %d, forwarded %d", len(log.Entries),
			len(forwarded))
	}
	for i, entry := range log.Entries {
		if entry.Observation.Entity.Name != forwarded[i] {
			t.Fatalf("entry %d recorded %s, forwarded %s", i,
				entry.Observation.Entity.Name, forwarded[i])
		}
	}
}
