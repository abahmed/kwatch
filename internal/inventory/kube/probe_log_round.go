package kube

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
)

const (
	// AttrProbeLogLine is the last line that names a failure in the
	// current log of a running container whose probe the application
	// answers with an HTTP error status. It is quoted, never
	// interpreted.
	AttrProbeLogLine = "probe.log.line"

	probeLogSource = "probe-log"
	// probeLogRecency is how recent the newest failed probe must be
	// for the container's log to be read.
	probeLogRecency = 5 * time.Minute
	// probeRereadRounds is how many rounds (5 minutes) pass before the
	// log of a container whose probe still fails is read again.
	probeRereadRounds = 10
	unhealthyReason   = "Unhealthy"
)

// CurrentLogReader reads the output of a container's current run.
type CurrentLogReader interface {
	CurrentRunLines(
		ctx context.Context, container inventory.EntityID,
	) ([]string, error)
}

// probeRead is the round's memory of one container's probe log: the
// round it was last read in, and the round a failed read may retry.
type probeRead struct {
	readAt, retryAt int
}

// probeAnswer matches the status code of an HTTP probe that got an
// answer ("HTTP probe failed with statuscode: 500", pkg/probe/http).
var probeAnswer = regexp.MustCompile(`statuscode: (\d{3})`)

// ProbeAnsweredStatus is the HTTP status a probe's application answered
// with, or zero when it did not answer (refused, timed out).
func ProbeAnsweredStatus(message string) int {
	m := probeAnswer.FindStringSubmatch(message)
	if m == nil {
		return 0
	}
	code, _ := strconv.Atoi(m[1])
	return code
}

// probePass reads the current log of the running containers whose
// newest failed probe the application answered with an HTTP error
// status: the application is up and says it is unwell, and its log
// usually says why. A probe that got no answer ("connection refused")
// means nothing listens, which the crash pass covers. Each container is
// read at most once per probeRereadRounds while its probe fails.
func (r *CrashLogRound) probePass(
	ctx context.Context, containers []inventory.EntityID, pass *readPass,
) []inventory.Observation {
	if r.cfg.Current == nil {
		return nil
	}
	now := r.cfg.Now()
	var observations []inventory.Observation
	for _, id := range containers {
		if !r.probeAnsweredError(id, now) {
			observations = append(observations, r.forgetProbe(id, now)...)
			continue
		}
		if !r.probeDue(id) {
			continue
		}
		if pass.spent(ctx) {
			break
		}
		pass.reads++
		lines, err := r.cfg.Current.CurrentRunLines(ctx, id)
		state := r.probes[id]
		if err != nil {
			if r.refused(err, containers, pass) {
				break
			}
			state.retryAt = r.rounds + errorBackoffRounds
			r.probes[id] = state
			continue
		}
		r.denied.Store(false)
		r.probes[id] = probeRead{readAt: r.rounds}
		observations = append(observations,
			ProbeLogObservation(id, now, lines))
	}
	return observations
}

// probeDue reports a container whose probe log was never read, or was
// read probeRereadRounds ago, and is not waiting after a failed read.
func (r *CrashLogRound) probeDue(id inventory.EntityID) bool {
	state, read := r.probes[id]
	if !read {
		return true
	}
	if state.retryAt > r.rounds {
		return false
	}
	return state.readAt == 0 || r.rounds-state.readAt >= probeRereadRounds
}

// forgetProbe removes the line of a container whose probe no longer
// gets an HTTP error status.
func (r *CrashLogRound) forgetProbe(
	id inventory.EntityID, now time.Time,
) []inventory.Observation {
	state, had := r.probes[id]
	if !had {
		return nil
	}
	delete(r.probes, id)
	if state.readAt == 0 {
		return nil
	}
	return []inventory.Observation{ProbeLogObservation(id, now, nil)}
}

// probeAnsweredError reports a running container whose newest failed
// probe, within probeLogRecency, got an HTTP error status.
func (r *CrashLogRound) probeAnsweredError(
	id inventory.EntityID, now time.Time,
) bool {
	e, ok := r.cfg.Model.Entity(id)
	if !ok {
		return false
	}
	if state, _ := e.Attribute(AttrState); state.Value.AsText() !=
		"running" {
		return false
	}
	var newest inventory.Note
	for _, note := range r.cfg.Model.Notes(id, now.Add(-probeLogRecency)) {
		if note.Warning && note.Reason == unhealthyReason &&
			!note.At.Before(newest.At) {
			newest = note
		}
	}
	return ProbeAnsweredStatus(newest.Message) >= 400
}

// ProbeLogObservation is what the round records for a running container
// whose probe gets an HTTP error status: the last line of its current
// log that names a failure, when there is one.
func ProbeLogObservation(
	id inventory.EntityID, at time.Time, lines []string,
) inventory.Observation {
	attrs := map[string]inventory.Value{}
	if line := LastFailureLine(lines); line != "" {
		attrs[AttrProbeLogLine] = inventory.Text(line)
	}
	return inventory.Observation{
		Kind: inventory.Observed, Source: probeLogSource,
		At: at, Entity: id, Attributes: attrs,
	}
}

// unwellLine matches what an application writes when its own health
// check fails: "unhealthy", "not ready", or a 5xx status it answered.
var unwellLine = regexp.MustCompile(`(?i)\bunhealthy\b|\bnot ready\b|` +
	`\b(status|code|http)[ =:/]*5\d\d\b`)

// LastFailureLine is the last line that names a failure and is not a
// stack frame. A running application says what is wrong with it now
// in its latest lines, so the last such line is the one quoted. It is
// quoted, never interpreted.
func LastFailureLine(lines []string) string {
	for i := len(lines) - 1; i >= 0; i-- {
		line := lines[i]
		if isTroubleLine(line) || (unwellLine.MatchString(line) &&
			!stackFrame.MatchString(line)) {
			return strings.TrimSpace(line)
		}
	}
	return ""
}
