package pipeline

import (
	"fmt"
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/storage"
)

// benchSize is the number of incidents and fingerprints a large cluster
// keeps: every save rewrote all of them before diff writes.
const benchSize = 5000

// openBenchStore opens a claimed store that closes with the benchmark.
func openBenchStore(b *testing.B) *storage.Store {
	b.Helper()
	s, err := storage.Open(b.TempDir()+"/state.db", storage.Options{
		Now: func() time.Time { return time.Unix(1_800_000_000, 0) },
	})
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = s.Close() })
	if _, err := s.Claim(); err != nil {
		b.Fatal(err)
	}
	return s
}

func benchRecords() []incident.Record {
	records := make([]incident.Record, benchSize)
	for i := range records {
		records[i] = incident.Record{
			ID:     fmt.Sprintf("incident-%05d", i),
			Root:   inventory.CoreID("pod", "default", fmt.Sprint(i)),
			Opened: time.Unix(1_800_000_000, 0).UTC(),
			Digest: fmt.Sprintf("digest-%d", i),
		}
	}
	return records
}

func benchFingerprints() map[string]any {
	values := make(map[string]any, benchSize)
	for i := 0; i < benchSize; i++ {
		values[fmt.Sprintf("pod/default/%05d", i)] = fmt.Sprint(i)
	}
	return values
}

// BenchmarkSaveIncidentsOneChanged saves 5k incidents where only one
// changed since the previous save, the common case for each batch.
func BenchmarkSaveIncidentsOneChanged(b *testing.B) {
	ps := NewIncidentStore(openBenchStore(b))
	records := benchRecords()
	if err := ps.SaveIncidents(records); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		records[0].Revision = i + 1
		if err := ps.SaveIncidents(records); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkSaveFingerprintsOneChanged saves 5k fingerprints where only
// one changed since the previous save.
func BenchmarkSaveFingerprintsOneChanged(b *testing.B) {
	ps := NewIncidentStore(openBenchStore(b))
	values := benchFingerprints()
	if err := ps.SaveFingerprints(values); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		values["pod/default/00000"] = fmt.Sprint("changed", i)
		if err := ps.SaveFingerprints(values); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkFlushFingerprintsOneChanged forces a write on every save, so
// it measures the diff write itself rather than the interval.
func BenchmarkFlushFingerprintsOneChanged(b *testing.B) {
	ps := NewIncidentStore(openBenchStore(b)).(*diskIncidents)
	values := benchFingerprints()
	if err := ps.SaveFingerprints(values); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		values["pod/default/00000"] = fmt.Sprint("changed", i)
		_ = ps.SaveFingerprints(values)
		if err := ps.FlushFingerprints(); err != nil {
			b.Fatal(err)
		}
	}
}
