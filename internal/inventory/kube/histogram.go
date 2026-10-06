package kube

import (
	"math"
	"sort"
	"strconv"
)

// histogram is one Prometheus histogram as the API server exposes it:
// cumulative bucket counts, so a bucket also counts every faster call.
// Subtracting an earlier reading of the same histogram leaves only the
// calls made in between, and the percentile of those is what the
// cluster felt since then, not its whole life.
type histogram struct {
	// bounds are the bucket upper limits in seconds, ascending, and
	// counts[i] is the number of observations at or below bounds[i].
	// The +Inf bucket is left out: count covers it.
	bounds []float64
	counts []float64
	sum    float64
	count  float64
}

// histogramReading collects the samples of one histogram family while
// its lines are read. Lines of one series arrive together, but several
// series may feed one histogram when the caller merges labels.
type histogramReading map[string]*histogramParts

type histogramParts struct {
	buckets    map[float64]float64
	sum, count float64
}

func (r histogramReading) parts(key string) *histogramParts {
	if r[key] == nil {
		r[key] = &histogramParts{buckets: map[float64]float64{}}
	}
	return r[key]
}

// add records one sample of a histogram family.
func (r histogramReading) add(key, suffix, le string, value float64) {
	if key == "" {
		return
	}
	parts := r.parts(key)
	switch suffix {
	case "_sum":
		parts.sum += value
	case "_count":
		parts.count += value
	default:
		if bound, err := strconv.ParseFloat(le, 64); err == nil &&
			!math.IsInf(bound, 1) {
			parts.buckets[bound] += value
		}
	}
}

// get is the histogram under key; it is empty when there is none.
func (r histogramReading) get(key string) histogram {
	if parts := r[key]; parts != nil {
		return parts.histogram()
	}
	return histogram{}
}

func (p *histogramParts) histogram() histogram {
	h := histogram{sum: p.sum, count: p.count}
	for bound := range p.buckets {
		h.bounds = append(h.bounds, bound)
	}
	sort.Float64s(h.bounds)
	for _, bound := range h.bounds {
		h.counts = append(h.counts, p.buckets[bound])
	}
	return h
}

// since is what was observed after an earlier reading of the same
// histogram. It is false when that cannot be told: the buckets differ,
// or a count went down, which means the process restarted.
func (h histogram) since(earlier histogram) (histogram, bool) {
	if len(h.bounds) != len(earlier.bounds) || h.count < earlier.count {
		return histogram{}, false
	}
	out := histogram{
		bounds: h.bounds, counts: make([]float64, len(h.counts)),
		sum: h.sum - earlier.sum, count: h.count - earlier.count,
	}
	for i := range h.counts {
		if h.bounds[i] != earlier.bounds[i] ||
			h.counts[i] < earlier.counts[i] {
			return histogram{}, false
		}
		out.counts[i] = h.counts[i] - earlier.counts[i]
	}
	return out, true
}

// merge adds the observations of other to h. Histograms of different
// shapes are not mixed: the one that is kept is the first.
func (h histogram) merge(other histogram) histogram {
	if h.count == 0 && len(h.bounds) == 0 {
		return other
	}
	if len(h.bounds) != len(other.bounds) {
		return h
	}
	out := histogram{
		bounds: h.bounds, counts: make([]float64, len(h.counts)),
		sum: h.sum + other.sum, count: h.count + other.count,
	}
	for i := range h.counts {
		out.counts[i] = h.counts[i] + other.counts[i]
	}
	return out
}

// quantile estimates the q-th quantile (0.99 for p99) in seconds, by
// the same straight-line guess inside a bucket that Prometheus makes.
// An answer past the last bucket is the last bucket's limit, a floor.
// It is false when nothing was observed.
func (h histogram) quantile(q float64) (float64, bool) {
	if h.count <= 0 || len(h.bounds) == 0 {
		return 0, false
	}
	rank := q * h.count
	lower, below := 0.0, 0.0
	for i, bound := range h.bounds {
		if h.counts[i] >= rank {
			in := h.counts[i] - below
			if in <= 0 {
				return bound, true
			}
			return lower + (bound-lower)*(rank-below)/in, true
		}
		lower, below = bound, h.counts[i]
	}
	return h.bounds[len(h.bounds)-1], true
}
