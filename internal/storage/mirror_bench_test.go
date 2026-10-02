package storage

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// benchItems is 5k values of about incident size.
func benchItems() map[string]Item[string] {
	items := make(map[string]Item[string], 5000)
	body := strings.Repeat("r", 400)
	for i := 0; i < 5000; i++ {
		items[fmt.Sprintf("incident-%05d", i)] = Item[string]{Value: body}
	}
	return items
}

func openBench(b *testing.B) *Store {
	b.Helper()
	s, err := Open(filepath.Join(b.TempDir(), "state.db"),
		Options{Now: newFakeClock().Now})
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = s.Close() })
	if _, err := s.Claim(); err != nil {
		b.Fatal(err)
	}
	return s
}

func pagesWritten(s *Store) int64 {
	stats := s.db.Stats()
	return stats.TxStats.GetWrite()
}

// reportPages reports the pages bbolt wrote per operation.
func reportPages(b *testing.B, s *Store, before int64) {
	written := pagesWritten(s) - before
	b.ReportMetric(float64(written)/float64(b.N), "pages/op")
}

// BenchmarkSaveOneChangedOf5k compares rewriting all 5k values on every
// save, as ReplaceAll did, with the Mirror's diff write.
func BenchmarkSaveOneChangedOf5k(b *testing.B) {
	b.Run("rewrite_all", func(b *testing.B) {
		s := openBench(b)
		values, items := IncidentRecords[string](s), benchItems()
		_ = values.PutAll(items)
		before := pagesWritten(s)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			items["incident-00000"] = Item[string]{Value: fmt.Sprint(i)}
			if err := values.PutAll(items); err != nil {
				b.Fatal(err)
			}
		}
		reportPages(b, s, before)
	})
	b.Run("diff", func(b *testing.B) {
		s := openBench(b)
		mirror, items := NewMirror(IncidentRecords[string](s)), benchItems()
		_, _ = mirror.Replace(items)
		before := pagesWritten(s)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			items["incident-00000"] = Item[string]{Value: fmt.Sprint(i)}
			if _, err := mirror.Replace(items); err != nil {
				b.Fatal(err)
			}
		}
		reportPages(b, s, before)
	})
}
