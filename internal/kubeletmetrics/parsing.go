package kubeletmetrics

import (
	"bufio"
	"strconv"
	"strings"
)

type counterPair struct{ Throttled, Periods float64 }

// parseCounters pairs CFS throttled and total period counters per container.
//
// cAdvisor keys its series by cgroup id, and a container that just restarted
// briefly exports two: the old cgroup, still present, and the new one. Keyed
// only by container, the last line read won for each metric independently, so
// throttled periods from one cgroup were divided by total periods from the
// other -- which is how a ratio came out at 600%. Series are therefore kept
// per id and one consistent pair chosen per container.
func parseCounters(body []byte) map[string]counterPair {
	byID := make(map[string]map[string]counterPair)
	scanner := bufio.NewScanner(strings.NewReader(string(body)))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "#") {
			continue
		}
		name, labels, value, ok := metricLine(line)
		if !ok ||
			(name != "container_cpu_cfs_throttled_periods_total" &&
				name != "container_cpu_cfs_periods_total") {
			continue
		}
		key := labels["namespace"] + "/" + labels["pod"] + "/" +
			labels["container"]
		pairs := byID[key]
		if pairs == nil {
			pairs = make(map[string]counterPair)
			byID[key] = pairs
		}
		pair := pairs[labels["id"]]
		if name == "container_cpu_cfs_throttled_periods_total" {
			pair.Throttled = value
		} else {
			pair.Periods = value
		}
		pairs[labels["id"]] = pair
	}
	out := make(map[string]counterPair, len(byID))
	for key, pairs := range byID {
		out[key] = liveCounterPair(pairs)
	}
	return out
}

// liveCounterPair picks, among the cgroups reporting for one container, the
// one that has accumulated the most scheduling periods -- the one that has
// actually been running. Both counters come from that same series.
func liveCounterPair(pairs map[string]counterPair) counterPair {
	var best counterPair
	for _, pair := range pairs {
		if pair.Periods > best.Periods {
			best = pair
		}
	}
	return best
}

func parseNamedCounters(body []byte, wanted string) map[string]float64 {
	out := make(map[string]float64)
	scanner := bufio.NewScanner(strings.NewReader(string(body)))
	for scanner.Scan() {
		name, labels, value, ok := metricLine(scanner.Text())
		if ok && name == wanted {
			out[labels["operation_type"]] += value
		}
	}
	return out
}

func sumCounters(values map[string]float64) float64 {
	var total float64
	for _, value := range values {
		total += value
	}
	return total
}

func metricLine(line string) (string, map[string]string, float64, bool) {
	parts := strings.Fields(line)
	if len(parts) < 2 {
		return "", nil, 0, false
	}
	value, err := strconv.ParseFloat(parts[1], 64)
	if err != nil {
		return "", nil, 0, false
	}
	name, labels := parts[0], map[string]string{}
	if open := strings.IndexByte(name, '{'); open >= 0 {
		labelText := strings.TrimSuffix(name[open+1:], "}")
		name = name[:open]
		for _, item := range strings.Split(labelText, ",") {
			pair := strings.SplitN(item, "=", 2)
			if len(pair) == 2 {
				labels[pair[0]] = strings.Trim(pair[1], "\"")
			}
		}
	}
	if labels["namespace"] == "" {
		labels["namespace"] = labels["pod_namespace"]
	}
	if labels["pod"] == "" {
		labels["pod"] = labels["pod_name"]
	}
	if labels["container"] == "" {
		labels["container"] = labels["container_name"]
	}
	return name, labels, value, true
}

func metricIdentity(key string) (string, string, string) {
	parts := strings.Split(key, "/")
	if len(parts) != 3 {
		return "", "", ""
	}
	return parts[0], parts[1], parts[2]
}
