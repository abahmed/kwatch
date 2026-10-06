package inventory

import (
	"math"
	"sort"
	"time"
)

// Stat is one rolling statistic: an exponentially weighted mean and the
// most recent samples, which serve as a small quantile sketch. Samples
// older than BaselineWindow are dropped, so the statistic describes the
// last week and nothing older.
type Stat struct {
	Count  int
	Mean   float64
	Recent []float64 `json:",omitempty"`
	// At holds when each Recent sample was taken, in Unix seconds. It
	// is empty for a statistic saved before samples were dated; such a
	// statistic ages out by sample count only.
	At []int64 `json:",omitempty"`
}

// add records value taken at at, keeping at most limit samples.
func (s *Stat) add(value float64, at time.Time, limit int) {
	if s.Count == 0 {
		s.Mean = value
	} else {
		s.Mean += ewmaAlpha * (value - s.Mean)
	}
	s.Count++
	if len(s.At) != len(s.Recent) {
		s.At = nil
	}
	s.Recent = append(s.Recent, value)
	s.At = append(s.At, at.Unix())
	s.trim(at, limit)
}

// trim drops samples older than the window before at, then the oldest
// ones beyond limit.
func (s *Stat) trim(at time.Time, limit int) {
	drop := 0
	if len(s.At) == len(s.Recent) {
		cutoff := at.Add(-BaselineWindow).Unix()
		for drop < len(s.At)-1 && s.At[drop] < cutoff {
			drop++
		}
	}
	drop = max(drop, len(s.Recent)-limit)
	if drop <= 0 {
		return
	}
	s.Recent = append(s.Recent[:0:0], s.Recent[drop:]...)
	if len(s.At) > drop {
		s.At = append(s.At[:0:0], s.At[drop:]...)
	} else {
		s.At = nil
	}
}

// Quantile returns the q-quantile (0..1) of the recent samples by the
// nearest-rank method, or 0 without samples.
func (s Stat) Quantile(q float64) float64 {
	return quantileOf(s.Recent, q)
}

func quantileOf(values []float64, q float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	rank := int(math.Ceil(q*float64(len(sorted)))) - 1
	return sorted[min(max(rank, 0), len(sorted)-1)]
}

// Median is the middle of the recent samples.
func (s Stat) Median() float64 { return s.Quantile(0.5) }

// MAD is the median absolute deviation of the recent samples from their
// median, scaled to match a standard deviation of normal data. Unlike a
// standard deviation, one wild hour does not widen it.
func (s Stat) MAD() float64 {
	if len(s.Recent) == 0 {
		return 0
	}
	median := s.Median()
	devs := make([]float64, len(s.Recent))
	for i, v := range s.Recent {
		devs[i] = math.Abs(v - median)
	}
	return madScale * quantileOf(devs, 0.5)
}

// High is the top of the statistic's normal range: the larger of the
// recent p95 and the median plus madSpread scaled MADs. A flat series
// has a range as narrow as its p95; a noisy one gets room to wobble.
func (s Stat) High() float64 {
	return math.Max(s.Quantile(0.95), s.Median()+madSpread*s.MAD())
}

// Typical is the figure to quote as "usual": the median, or the mean
// when most samples are zero, so a workload that restarts about once a
// day reads "0.04 a hour" and not "0".
func (s Stat) Typical() float64 {
	if median := s.Median(); median > 0 || len(s.Recent) == 0 {
		return median
	}
	sum := 0.0
	for _, v := range s.Recent {
		sum += v
	}
	return sum / float64(len(s.Recent))
}
