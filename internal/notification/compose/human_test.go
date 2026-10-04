package compose

import (
	"testing"
	"time"
)

func TestHumanDurationRoundsLikePeople(t *testing.T) {
	tests := map[time.Duration]string{
		30 * time.Second:                "less than a minute",
		7*time.Minute + 10*time.Second:  "seven minutes",
		90 * time.Second:                "about two minutes",
		time.Hour:                       "one hour",
		2*time.Hour + 30*time.Minute:    "about three hours",
		6 * 24 * time.Hour:              "six days",
		13 * 24 * time.Hour:             "13 days",
		14*24*time.Hour + 2*time.Minute: "two weeks",
		21 * 24 * time.Hour:             "three weeks",
	}
	for d, want := range tests {
		if got := humanDuration(d); got != want {
			t.Errorf("humanDuration(%s) = %q, want %q", d, got, want)
		}
	}
}

func TestHumanBytes(t *testing.T) {
	tests := map[string]string{
		"11534Mi":     "about 11 GiB",
		"512Mi":       "512 MiB",
		"2147483648":  "2 GiB",
		"96%":         "96%",
		"12":          "12",
		"not-a-value": "not-a-value",
	}
	for in, want := range tests {
		if got := humanBytes(in); got != want {
			t.Errorf("humanBytes(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestHumanizeTextRewritesDurations(t *testing.T) {
	tests := map[string]string{
		"TLS certificate expires in 72h0m0s": "TLS certificate " +
			"expires in three days",
		"Pod has not been ready for 5m30s": "Pod has not been ready " +
			"for six minutes",
		"3 of 4 pods fail":  "3 of 4 pods fail",
		"took 20ms to fail": "took 20ms to fail",
	}
	for in, want := range tests {
		if got := humanizeText(in); got != want {
			t.Errorf("humanizeText(%q) = %q, want %q", in, got, want)
		}
	}
}

// Resource quantities, names and versions share the duration suffixes;
// only a token shaped like a Go duration is rewritten.
func TestHumanizeTextLeavesQuantitiesAlone(t *testing.T) {
	tests := map[string]string{
		"cpu limit 500m exceeded": "cpu limit 500m exceeded",
		"limit 250m cores":        "limit 250m cores",
		"pod x-12h restarted":     "pod x-12h restarted",
		"version 1.2m is old":     "version 1.2m is old",
		"cpu=500m memory=1Gi":     "cpu=500m memory=1Gi",
		"waited 12h for it":       "waited 12h for it",
		"job x-12h0m0s failed":    "job x-12h0m0s failed",
		"revision 1.5m30s":        "revision 1.5m30s",
		"certificate expires in 72h0m0s.": "certificate expires in " +
			"three days.",
		"not ready for 5m30s, restarting": "not ready for six minutes, " +
			"restarting",
		"took 2h30m to finish": "took about three hours to finish",
		"45s and 5m30s":        "less than a minute and six minutes",
		"only 45s":             "only less than a minute",
	}
	for in, want := range tests {
		if got := humanizeText(in); got != want {
			t.Errorf("humanizeText(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestJoinAndCountsTheRest(t *testing.T) {
	got := joinAnd([]string{"a", "b", "c", "d", "e"}, 3)
	if want := "a, b, c and two others"; got != want {
		t.Errorf("joinAnd = %q, want %q", got, want)
	}
}
