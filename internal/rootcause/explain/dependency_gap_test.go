package explain

import (
	"math/rand"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"
)

// referenceBreak and referenceGap are the gap check as a scan of the
// text between a name and a failure, which failureSet must match.
var referenceBreak = regexp.MustCompile(`[;\n]|,\s`)

func referenceGap(lower string, from, to int) (int, bool) {
	gap := lower[from:to]
	if referenceBreak.MatchString(gap) {
		return 0, false
	}
	return len(strings.Fields(gap)), true
}

// The indexed gap check counts words and finds clause breaks exactly
// as a scan of the text between would.
func TestFailureSetMatchesAScan(t *testing.T) {
	pieces := []string{"a", "redis", " ", "  ", "\t", "\n", ";", ",",
		", ", ",1", "é", "x y", "refused", ":", "12:30:45,123"}
	rng := rand.New(rand.NewSource(7))
	for range 2000 {
		var b strings.Builder
		for range rng.Intn(24) {
			b.WriteString(pieces[rng.Intn(len(pieces))])
		}
		lower := b.String()
		fs := newFailureSet(lower, nil)
		var cuts []int
		for i := range lower {
			cuts = append(cuts, i)
		}
		cuts = append(cuts, len(lower))
		for _, from := range cuts {
			for _, to := range cuts {
				if from > to {
					continue
				}
				words, ok := referenceGap(lower, from, to)
				broken := fs.broken(from, to)
				if broken != !ok || (ok && fs.words(lower, from, to) != words) {
					t.Fatalf("%q[%d:%d]: index (%v, %d), scan (%v, %d)",
						lower, from, to, !broken,
						fs.words(lower, from, to), ok, words)
				}
			}
		}
	}
}

// A retry loop repeating a failure and a client name thousands of
// times grows near-linearly, not as a scan per pair did (4 KB: 1.5 s,
// 12 KB: 40 s), and the scan stops at its cap. Checked as growth
// between sizes, not wall time, so slow or busy machines pass.
func TestDependencyInIsFastOnRepeatedText(t *testing.T) {
	for _, unit := range []string{"connection refusedmongodb",
		"redis connection refused ", "refused redis ", "kafka refused "} {
		at := func(size int) time.Duration {
			text := strings.Repeat(unit, size/len(unit)+1)[:size]
			return medianOf(func() { DependencyIn(text) })
		}
		small, cap1k, big := at(256), at(1<<10), at(12<<10)
		slack := 2 * time.Millisecond
		if cap1k > 16*small+slack {
			t.Errorf("%q: 1 KB took %v, 256 B %v: not near-linear",
				unit, cap1k, small)
		}
		if big > 3*cap1k+slack {
			t.Errorf("%q: 12 KB took %v, 1 KB %v: the cap does not hold",
				unit, big, cap1k)
		}
	}
}

// medianOf times f five times and returns the median.
func medianOf(f func()) time.Duration {
	runs := make([]time.Duration, 5)
	for i := range runs {
		start := time.Now()
		f()
		runs[i] = time.Since(start)
	}
	slices.Sort(runs)
	return runs[len(runs)/2]
}
