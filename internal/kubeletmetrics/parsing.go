package kubeletmetrics

import (
	"bufio"
	"strconv"
	"strings"
)

type counterPair struct{ Throttled, Periods float64 }

func parseCounters(body []byte) map[string]counterPair {
	out := make(map[string]counterPair)
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
		key := labels["namespace"] + "/" + labels["pod"] + "/" + labels["container"]
		pair := out[key]
		if name == "container_cpu_cfs_throttled_periods_total" {
			pair.Throttled = value
		} else {
			pair.Periods = value
		}
		out[key] = pair
	}
	return out
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
