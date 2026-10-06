package detectors

import (
	"fmt"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/format"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// Memory history judgements. They describe the past run only.
const (
	// quickKill is how soon after starting a kill counts as "within
	// seconds of starting": the container needs more than its limit as
	// soon as it runs.
	quickKill = 2 * time.Minute
	// riseMinRun is the shortest run whose climb is worth describing.
	riseMinRun = 30 * time.Minute
	// riseFactor is how much higher than where it began the run ended.
	riseFactor = 1.5
	// riseDrawdownPct is the largest fall, in percent of the highest
	// use so far, a climb may have and still be steady.
	riseDrawdownPct = 10.0
	// runMatchGap is how far apart the last memory reading of the
	// previous run and the kill may be and still be the same run.
	runMatchGap = 5 * time.Minute
)

func killedByMemory(e inventory.Entity) bool {
	return text(e, kube.AttrLastReason) == reasons.OOMKilled ||
		text(e, kube.AttrStateReason) == reasons.OOMKilled
}

// memoryEvidence says what the container used before an OOM kill:
// the limit and the peak of the last day, a steady climb to the kill,
// or a kill soon after starting. Without usage history it adds only
// what the termination itself shows.
func memoryEvidence(e inventory.Entity) []detection.Evidence {
	var out []detection.Evidence
	add := func(label, value string) {
		out = append(out, detection.Evidence{Label: label, Value: value})
	}
	if lived, ok := livedBeforeKill(e); ok {
		add(detection.EvidenceKilledAfter, seconds(lived))
	}
	peak, ok := number(e, kube.AttrMemoryPeak24h)
	if !ok || peak <= 0 {
		return out
	}
	if limit, ok := number(e, kube.AttrMemoryLimit); ok && limit > 0 {
		add(detection.EvidenceMemoryLimit, quantity(limit))
	}
	add(detection.EvidenceMemoryPeak, quantity(peak))
	if rise, ok := steadyRise(e); ok {
		add(detection.EvidenceMemoryRise, rise)
	}
	return out
}

// livedBeforeKill is how long the killed run lasted, when it was
// killed soon after it started.
func livedBeforeKill(e inventory.Entity) (time.Duration, bool) {
	started := timestamp(e, kube.AttrLastStarted)
	finished := timestamp(e, kube.AttrLastFinished)
	if started.IsZero() || finished.IsZero() {
		return 0, false
	}
	lived := finished.Sub(started)
	if lived < 0 || lived > quickKill {
		return 0, false
	}
	return max(lived, time.Second), true
}

// prevRun is the memory history of the run before the kill, as the
// inventory recorded it.
type prevRun struct {
	start, peak float64
	ended       time.Time
	length      time.Duration
	// drawdown is the largest fall in percent of the highest use so far.
	drawdown float64
}

func prevRunOf(e inventory.Entity) prevRun {
	r := prevRun{ended: timestamp(e, kube.AttrMemoryPrevEnded)}
	r.start, _ = number(e, kube.AttrMemoryPrevStart)
	r.peak, _ = number(e, kube.AttrMemoryPrevPeak)
	r.drawdown, _ = number(e, kube.AttrMemoryPrevDrawdown)
	secs, _ := number(e, kube.AttrMemoryPrevSeconds)
	r.length = time.Duration(secs * float64(time.Second))
	return r
}

// steadyRise describes a run that climbed without falling back until
// it was killed: "200Mi to 512Mi over 3h".
func steadyRise(e inventory.Entity) (string, bool) {
	run := prevRunOf(e)
	finished := timestamp(e, kube.AttrLastFinished)
	gap := finished.Sub(run.ended)
	if run.ended.IsZero() || finished.IsZero() || gap > runMatchGap ||
		gap < -runMatchGap {
		return "", false
	}
	if run.start <= 0 || run.peak < riseFactor*run.start ||
		run.length < riseMinRun || run.drawdown > riseDrawdownPct {
		return "", false
	}
	return quantity(run.start) + " to " + quantity(run.peak) + " over " +
		format.Duration(run.length), true
}

// quantity writes bytes the way a memory limit is written:
// "512Mi", "1.5Gi".
func quantity(bytes float64) string {
	const mebi, gibi = 1 << 20, 1 << 30
	if bytes >= gibi {
		value := fmt.Sprintf("%.1f", bytes/gibi)
		return strings.TrimSuffix(value, ".0") + "Gi"
	}
	return fmt.Sprintf("%.0fMi", bytes/mebi)
}
