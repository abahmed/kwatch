package kube

import (
	"bufio"
	"bytes"
	"strconv"
	"strings"
	"sync"
	"time"
)

// counterRates turns monotonically increasing counters into per-second
// rates between polls. A counter that went backwards (a restart) resets.
type counterRates struct {
	mu       sync.Mutex
	previous map[string]counterSample
}

type counterSample struct {
	at    time.Time
	value float64
}

func newCounterRates() *counterRates {
	return &counterRates{previous: map[string]counterSample{}}
}

// rate records value for key and returns the rate since the last sample.
func (c *counterRates) rate(
	key string, at time.Time, value float64,
) (float64, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	previous, ok := c.previous[key]
	c.previous[key] = counterSample{at: at, value: value}
	seconds := at.Sub(previous.at).Seconds()
	if !ok || seconds <= 0 || value < previous.value {
		return 0, false
	}
	return (value - previous.value) / seconds, true
}

// throttleRatios parses cAdvisor output into the share of CFS periods
// each container spent throttled since the previous poll. cAdvisor may
// report two cgroups for a just-restarted container; the one with the
// most periods is the live one.
func (c *counterRates) throttleRatios(
	body []byte, at time.Time,
) map[string]float64 {
	type pair struct{ throttled, periods float64 }
	byID := map[string]map[string]pair{}
	scanner := bufio.NewScanner(bytes.NewReader(body))
	scanner.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for scanner.Scan() {
		name, labels, value, ok := metricLine(scanner.Text())
		if !ok || (name != "container_cpu_cfs_throttled_periods_total" &&
			name != "container_cpu_cfs_periods_total") {
			continue
		}
		key := labels["namespace"] + "/" + labels["pod"] + "/" +
			labels["container"]
		if byID[key] == nil {
			byID[key] = map[string]pair{}
		}
		p := byID[key][labels["id"]]
		if name == "container_cpu_cfs_periods_total" {
			p.periods = value
		} else {
			p.throttled = value
		}
		byID[key][labels["id"]] = p
	}
	out := map[string]float64{}
	for key, pairs := range byID {
		var live pair
		for _, p := range pairs {
			if p.periods > live.periods {
				live = p
			}
		}
		throttled, ok1 := c.rate("throttled/"+key, at, live.throttled)
		periods, ok2 := c.rate("periods/"+key, at, live.periods)
		if ok1 && ok2 && periods > 0 {
			out[key] = 100 * throttled / periods
		}
	}
	return out
}

// sumMetric adds every sample of one metric name.
func sumMetric(body []byte, wanted string) (float64, bool) {
	total, found := 0.0, false
	scanner := bufio.NewScanner(bytes.NewReader(body))
	scanner.Buffer(make([]byte, 0, 64<<10), 1<<20)
	for scanner.Scan() {
		if name, _, value, ok := metricLine(scanner.Text()); ok &&
			name == wanted {
			total += value
			found = true
		}
	}
	return total, found
}

// metricLine parses one Prometheus text-format sample.
func metricLine(line string) (string, map[string]string, float64, bool) {
	if line == "" || strings.HasPrefix(line, "#") {
		return "", nil, 0, false
	}
	name, rest := line, ""
	labels := map[string]string{}
	if open := strings.IndexByte(line, '{'); open >= 0 {
		end := strings.LastIndexByte(line, '}')
		if end < open {
			return "", nil, 0, false
		}
		name, rest = line[:open], line[end+1:]
		for _, pair := range splitLabels(line[open+1 : end]) {
			key, value, ok := strings.Cut(pair, "=")
			if ok {
				labels[key] = strings.Trim(value, `"`)
			}
		}
	} else if space := strings.IndexByte(line, ' '); space >= 0 {
		name, rest = line[:space], line[space:]
	}
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return "", nil, 0, false
	}
	value, err := strconv.ParseFloat(fields[0], 64)
	return name, labels, value, err == nil
}

// splitLabels splits a label set on commas outside quotes.
func splitLabels(set string) []string {
	var out []string
	quoted, start := false, 0
	for i, r := range set {
		switch {
		case r == '"' && (i == 0 || set[i-1] != '\\'):
			quoted = !quoted
		case r == ',' && !quoted:
			out = append(out, set[start:i])
			start = i + 1
		}
	}
	if start < len(set) {
		out = append(out, set[start:])
	}
	return out
}
