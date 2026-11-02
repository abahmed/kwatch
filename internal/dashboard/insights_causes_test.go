package dashboard

import (
	"strings"
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
)

func TestModeLabelKeepsAcronymsWhole(t *testing.T) {
	cases := map[string]string{
		"APIServer":             "API server",
		"APIServiceUnavailable": "API service unavailable",
		"CPUHigh":               "CPU high",
		"CPUThrottled":          "CPU throttled",
		"CRD":                   "CRD",
		"CSRFailed":             "CSR failed",
		"DeprecatedAPI":         "Deprecated API",
		"PLEGSlow":              "PLEG slow",
		"PIDPressure":           "PID pressure",
		"RolloutStuck":          "Stuck rollouts",
		"LoadBalancerPending":   "Load balancer pending",
		"OOMKilled":             "Out of memory",
		"Etcd":                  "Etcd",
		"Node2Down":             "Node2 down",
	}
	for family, want := range cases {
		if got := modeLabel(family); got != want {
			t.Errorf("modeLabel(%q) = %q, want %q", family, got, want)
		}
	}
}

// Every family a finding can carry reads as words: no letter stands
// alone and no acronym is broken up.
func TestModeLabelReadsAsWordsForEveryFamily(t *testing.T) {
	families := map[string]bool{}
	for _, e := range detection.Modes() {
		families[string(e.Mode.Family())] = true
	}
	for _, m := range []detection.Mode{
		detection.ModeMemoryEvictionThreshold,
		detection.ModePIDEvictionThreshold, detection.ModeFailedCreate,
		detection.ModeTerminating, detection.ModeUnreachable,
		detection.ModeNodeCondition, detection.ModeQuota,
	} {
		families[string(m.Family())] = true
	}
	if len(families) < 50 {
		t.Fatalf("only %d families", len(families))
	}
	for family := range families {
		label := modeLabel(family)
		if label == "" || strings.ToUpper(label[:1]) != label[:1] {
			t.Errorf("modeLabel(%q) = %q: not capitalised", family,
				label)
		}
		for _, w := range strings.Fields(label) {
			if len([]rune(w)) == 1 && w != "a" {
				t.Errorf("modeLabel(%q) = %q: lone letter %q",
					family, label, w)
			}
		}
	}
}

func TestModeLabelsOfNamesEachRequestedMode(t *testing.T) {
	if modeLabelsOf(nil) != nil {
		t.Error("no modes, want nil")
	}
	got := modeLabelsOf(map[string]bool{"CPUHigh": true,
		"ImagePull": true})
	if got["CPUHigh"] != "CPU high" ||
		got["ImagePull"] != "Image pull failures" {
		t.Errorf("labels = %v", got)
	}
}
