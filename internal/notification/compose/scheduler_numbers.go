package compose

import (
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
)

// weightSchedulerNumbers puts the numbers before the scheduler's quote:
// they say what is missing, the quote only repeats that it is.
const weightSchedulerNumbers = weightScheduler + 0.1

// noSchedulableNode is the "most free" value when no node can run pods.
const noSchedulableNode = "no schedulable node"

// schedulerNumbers say what an unschedulable pod needs and the most any
// node has free: "It needs 3 CPU; the most any node has free is 1.5 CPU,
// on n1." Nothing when the finding carries no quantities.
func schedulerNumbers(f caseFacts) []sentence {
	for _, s := range f.members {
		if text := needsText(s); text != "" {
			return []sentence{{part: partProof,
				weight: weightSchedulerNumbers, text: text}}
		}
	}
	return nil
}

// needsText reads the finding's "needs" and "most free" evidence pairs,
// CPU first and memory second, in the order they were recorded.
func needsText(s detection.Finding) string {
	var needs, free []string
	for _, e := range s.Evidence {
		switch value := strings.TrimSpace(e.Value); {
		case value == "":
		case e.Label == "needs":
			needs = append(needs, value)
		case e.Label == "most free":
			free = append(free, value)
		}
	}
	if len(needs) == 0 || len(needs) != len(free) {
		return ""
	}
	text := "It needs " + strings.Join(needs, " and ")
	for _, value := range free {
		if value == noSchedulableNode {
			return text + "; no node can take it, none are schedulable."
		}
	}
	return text + "; the most any node has free is " + freeWords(free) + "."
}

// freeWords words each "1.5 CPU on n1" as "1.5 CPU, on n1" and joins
// pairs with "and".
func freeWords(free []string) string {
	words := make([]string, 0, len(free))
	for _, value := range free {
		if i := strings.LastIndex(value, " on "); i >= 0 {
			value = value[:i] + ", on " + value[i+len(" on "):]
		}
		words = append(words, value)
	}
	return strings.Join(words, " and ")
}
