package kube

import (
	"context"
	"errors"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	corev1 "k8s.io/api/core/v1"

	"github.com/abahmed/kwatch/internal/inventory"
)

// Attributes the log sample round writes on a workload. They describe
// the last sample window of one pod's log and the windows before it.
const (
	// AttrLogErrors is the error lines in the last window.
	AttrLogErrors = "log.errors"
	// AttrLogWindow is the length of the window, in seconds.
	AttrLogWindow = "log.window.seconds"
	// AttrLogBaseline is the error lines in the windows before it.
	AttrLogBaseline = "log.baseline.errors"
	// AttrLogBaselineSpan is how many seconds those windows cover.
	AttrLogBaselineSpan = "log.baseline.seconds"
	// AttrLogTopLine is the most frequent error line, as first seen.
	AttrLogTopLine = "log.top.line"
	// AttrLogTopCount is how many times that line came up.
	AttrLogTopCount = "log.top.count"
	// AttrLogTopBaseline is how often it came up in the earlier windows.
	AttrLogTopBaseline = "log.top.baseline"
)

const (
	// logSampleSource carries the sample of a workload's healthy pods.
	logSampleSource = "log-sample"
	// logSampleWindow is how much log one sample covers; each workload
	// is sampled once per window.
	logSampleWindow = 2 * time.Minute
	// logSampleHistory is how far back earlier windows count as the
	// workload's own baseline.
	logSampleHistory = time.Hour
	// logSampleTail and logSampleBytes bound one read.
	logSampleTail  = 1000
	logSampleBytes = 256 << 10
	// maxQuotedLine is the longest error line kept, as the spec quotes.
	maxQuotedLine = 120
	// maxTrackedLines bounds the distinct error lines one window keeps.
	maxTrackedLines = 20
)

// digitRun and hexRun fold the parts of a line that change on every
// repeat (timestamps, ids, counts) so repeats of one error group.
var (
	digitRun = regexp.MustCompile(`\d+`)
	hexRun   = regexp.MustCompile(`\b[0-9a-f]{8,}\b`)
)

// windowCount is the error lines of one sample window.
type windowCount struct {
	at     time.Time
	errors int
	// lines counts each distinct error line by its folded form.
	lines map[string]int
	// shown is the line as first seen, per folded form.
	shown map[string]string
	// capped is set when the window met more distinct lines than it
	// tracks: a line it does not list may still have come up in it.
	capped bool
}

// countErrors counts the error lines of a window and groups repeats.
func countErrors(lines []string, at time.Time) windowCount {
	w := windowCount{at: at, lines: map[string]int{},
		shown: map[string]string{}}
	for _, line := range lines {
		if !IsErrorLine(line) {
			continue
		}
		w.errors++
		key := foldLine(line)
		if _, tracked := w.lines[key]; !tracked &&
			len(w.lines) >= maxTrackedLines {
			w.capped = true
			continue
		}
		w.lines[key]++
		if _, seen := w.shown[key]; !seen {
			w.shown[key] = clipLine(line)
		}
	}
	return w
}

func foldLine(line string) string {
	return digitRun.ReplaceAllString(
		hexRun.ReplaceAllString(strings.ToLower(line), "#"), "#")
}

// clipLine cuts a line to maxQuotedLine runes.
func clipLine(line string) string {
	line = strings.TrimSpace(line)
	if utf8.RuneCountInString(line) <= maxQuotedLine {
		return line
	}
	return string([]rune(line)[:maxQuotedLine])
}

// top is the most frequent folded line; ties go to the smaller form.
func (w windowCount) top() (key string, count int) {
	keys := make([]string, 0, len(w.lines))
	for k := range w.lines {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if w.lines[k] > count {
			key, count = k, w.lines[k]
		}
	}
	return key, count
}

// sampleObservation describes the last window against the earlier ones.
func sampleObservation(
	workload inventory.EntityID, now time.Time, last windowCount,
	earlier []windowCount,
) inventory.Observation {
	baseline, topBaseline := 0, 0
	key, count := last.top()
	unknown := false
	for _, w := range earlier {
		baseline += w.errors
		topBaseline += w.lines[key]
		// A line a capped window does not list may have come up in it:
		// its earlier count is unknown, not zero.
		if _, listed := w.lines[key]; !listed && w.capped {
			unknown = true
		}
	}
	attrs := map[string]inventory.Value{
		AttrLogErrors:   inventory.Number(float64(last.errors)),
		AttrLogWindow:   inventory.Number(logSampleWindow.Seconds()),
		AttrLogBaseline: inventory.Number(float64(baseline)),
		AttrLogBaselineSpan: inventory.Number(
			float64(len(earlier)) * logSampleWindow.Seconds()),
	}
	if count > 0 {
		attrs[AttrLogTopLine] = inventory.Text(last.shown[key])
		attrs[AttrLogTopCount] = inventory.Number(float64(count))
		if !unknown {
			attrs[AttrLogTopBaseline] = inventory.Number(
				float64(topBaseline))
		}
	}
	return inventory.Observation{
		Kind: inventory.Observed, Source: logSampleSource, At: now,
		Entity: workload, Attributes: attrs,
	}
}

// SampleLines returns the last since of a running container's output,
// at most logSampleTail lines and logSampleBytes bytes, redacted and
// bounded like Lines. An error means the read failed (a Forbidden one
// means pods/log is not granted).
func (r LogReader) SampleLines(
	ctx context.Context, container inventory.EntityID, since time.Duration,
) ([]string, error) {
	pod, name, ok := strings.Cut(container.Name, "/")
	if !ok || r.Client == nil {
		return nil, errors.New("no log client or container name")
	}
	fetchCtx, cancel := context.WithTimeout(ctx, logFetchBudget)
	defer cancel()
	seconds, tail, limit := int64(since.Seconds()), int64(logSampleTail),
		int64(logSampleBytes)
	body, err := r.Client.CoreV1().Pods(container.Namespace).GetLogs(
		pod, &corev1.PodLogOptions{Container: name, SinceSeconds: &seconds,
			TailLines: &tail, LimitBytes: &limit}).DoRaw(fetchCtx)
	if err != nil {
		return nil, err
	}
	return logLines(body), nil
}

// LogSampleObservation describes a workload from the log lines of
// consecutive sample windows, oldest first, the last window against the
// ones before it. It is the observation LogSampleRound records; tools
// that replay a log use it to say the same thing.
func LogSampleObservation(
	workload inventory.EntityID, at time.Time, windows [][]string,
) inventory.Observation {
	if len(windows) == 0 {
		return inventory.Observation{Kind: inventory.Observed,
			Source: logSampleSource, At: at, Entity: workload}
	}
	counted := make([]windowCount, len(windows))
	for i, lines := range windows {
		counted[i] = countErrors(lines, at)
	}
	last := counted[len(counted)-1]
	return sampleObservation(workload, at, last, counted[:len(counted)-1])
}
