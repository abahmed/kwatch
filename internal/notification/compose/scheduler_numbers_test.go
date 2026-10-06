package compose

import (
	"testing"

	"github.com/abahmed/kwatch/internal/detection"
)

func pair(label, value string) detection.Evidence {
	return detection.Evidence{Label: label, Value: value}
}

func TestNeedsTextWordsTheNumbers(t *testing.T) {
	cpuAndMemory := detection.Finding{Evidence: []detection.Evidence{
		pair("needs", "3 CPU"), pair("most free", "1.5 CPU on n1"),
		pair("needs", "4 GiB memory"),
		pair("most free", "2.5 GiB memory on n2")}}
	none := detection.Finding{Evidence: []detection.Evidence{
		pair("needs", "3 CPU"), pair("most free", noSchedulableNode)}}

	cases := map[string]detection.Finding{
		"It needs 3 CPU and 4 GiB memory; the most any node has free " +
			"is 1.5 CPU, on n1 and 2.5 GiB memory, on n2.": cpuAndMemory,
		"It needs 3 CPU; no node can take it, none are schedulable.": none,
		"": {},
	}
	for want, finding := range cases {
		if got := needsText(finding); got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	}
}
