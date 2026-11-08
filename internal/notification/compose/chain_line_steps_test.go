package compose

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/rootcause"
)

func hopAt(kind inventory.Kind, ns, name string, h, m int) rootcause.Hop {
	return rootcause.Hop{Entity: inventory.CoreID(kind, ns, name),
		Began: time.Date(2026, 11, 1, h, m, 0, 0, time.UTC)}
}

func finding(kind inventory.Kind, ns, name string, mode detection.Mode,
) detection.Finding {
	return detection.Finding{Entity: inventory.CoreID(kind, ns, name),
		Mode: mode, Severity: detection.Critical}
}

func TestTimelineIsOrderedByOnsetAndJoinsOneMinuteOneEvent(t *testing.T) {
	cause := &rootcause.CauseRecord{Hops: []rootcause.Hop{
		// Out of order on purpose: the line follows the clock.
		hopAt(kube.KindDeployment, "shop", "redis", 14, 3),
		hopAt(kube.KindDeployment, "shop", "checkout", 14, 3),
		hopAt(kube.KindNode, "", "n3", 14, 2),
		hopAt(kube.KindDeployment, "shop", "cart", 14, 3),
	}}
	f := caseFacts{p: incident.Incident{Cause: cause},
		members: []detection.Finding{
			finding(kube.KindNode, "", "n3", detection.ModeDiskPressure),
			finding(kube.KindContainer, "shop", "redis-7d9/app",
				detection.ModeEvicted),
			finding(kube.KindContainer, "shop", "checkout-6d4/app",
				detection.ModeCrashLoop),
			finding(kube.KindContainer, "shop", "cart-9b1/app",
				detection.ModeCrashLoop),
		}}

	got := chainLineSentences(f)

	if assert.Len(t, got, 1) {
		assert.Equal(t, "Timeline: 14:02 n3 disk pressure → "+
			"14:03 redis evicted → 14:03 checkout, cart crash.",
			got[0].text)
	}
}

func TestTimelineNamesAtMostTwoThenCounts(t *testing.T) {
	step := timelineStep{minute: "14:03", event: "crash",
		names: []string{"a", "b", "c", "d"}}
	assert.Equal(t, "14:03 a, b +2 crash", step.words())
	unknown := timelineStep{event: "crash", names: []string{"a"}}
	assert.Equal(t, "a crash", unknown.words(), "no time is not invented")
}

func TestTimelineLeftOutOfALongMessage(t *testing.T) {
	long := []sentence{{text: strings.Repeat("x", 590)},
		{fact: factTimeline, text: "Timeline: 14:02 a → b → c."}}
	assert.Len(t, trimTimeline(long), 1)
	short := []sentence{{text: "short"},
		{fact: factTimeline, text: "Timeline: a."}}
	assert.Len(t, trimTimeline(short), 2)
}
