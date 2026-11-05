package detectors

import (
	"regexp"
	"strconv"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// probeStatus matches the status code of an HTTP probe that got an
// answer ("HTTP probe failed with statuscode: 500", pkg/probe/http).
var probeStatus = regexp.MustCompile(`statuscode: (\d{3})`)

// probeResultClasses say in a few words how a probe failed when it got
// no answer, from the kubelet's message; the first match wins.
var probeResultClasses = []struct {
	pattern *regexp.Regexp
	words   string
}{
	{regexp.MustCompile(`connection refused`), "connection refused"},
	{regexp.MustCompile(`deadline exceeded|timed out|timeout`),
		"timed out"},
	{regexp.MustCompile(`connection reset`), "connection reset"},
	{regexp.MustCompile(`no route to host`), "no route to host"},
}

// probeResult is how a probe failed, in a few words: "HTTP 500" when
// the application answered, else the class of the failure. Empty when
// the message says neither.
func probeResult(message string) string {
	if m := probeStatus.FindStringSubmatch(message); m != nil {
		return "HTTP " + m[1]
	}
	for _, class := range probeResultClasses {
		if class.pattern.MatchString(message) {
			return class.words
		}
	}
	return ""
}

// answeredStatus is the HTTP status a probe's application answered
// with, or zero when it did not answer.
func answeredStatus(message string) int {
	return kube.ProbeAnsweredStatus(message)
}

// probeLogEvidence is the line the container's current log names its
// failure with, while its probe gets an HTTP error status. A probe that
// got no answer has no such line: nothing was listening to write it.
func probeLogEvidence(
	e inventory.Entity, note inventory.Note,
) []detection.Evidence {
	line := text(e, kube.AttrProbeLogLine)
	if line == "" || answeredStatus(note.Message) < 400 {
		return nil
	}
	return []detection.Evidence{{
		Label: detection.EvidenceProbeLog, Value: line}}
}

// probeResultEvidence is the probe result of a note, when it says one.
func probeResultEvidence(note inventory.Note) []detection.Evidence {
	words := probeResult(note.Message)
	if words == "" {
		return nil
	}
	return []detection.Evidence{{
		Label: detection.EvidenceProbeResult, Value: words}}
}

// crashedByItself reports a container whose last run ended on its own
// with an error: a non-zero exit that is not a kill (SIGTERM or SIGKILL
// from the kubelet), not a memory kill, and not the program's own exit
// code after the kubelet stopped it for its liveness probe (an app may
// exit 1 on SIGTERM). Its probes fail because the process is gone or
// not listening yet, so they are a symptom.
func crashedByItself(ctx detection.Context, e inventory.Entity) bool {
	code, known := number(e, kube.AttrLastExitCode)
	return known &&
		reasons.CrashExit(int(code), text(e, kube.AttrLastReason)) &&
		!livenessKillAtExit(ctx, e)
}

// probeSummary is what a failing probe of a container says. A liveness
// probe of a container that crashed by itself fails because nothing
// listens while it is down: the crash is the problem, so it says that.
// A readiness check that the running application answers with an error
// status is the application's own verdict on itself, so it says so with
// the status.
func probeSummary(ctx detection.Context, e inventory.Entity,
	note inventory.Note, reason, fallback string,
) string {
	if reason == reasons.LivenessProbeFailed && crashedByItself(ctx, e) {
		code, _ := number(e, kube.AttrLastExitCode)
		return containerRole(e) + " keeps exiting on its own with an " +
			"error (exit code " + strconv.Itoa(int(code)) + ")"
	}
	code := answeredStatus(note.Message)
	if reason != reasons.ReadinessProbeFailed || code < 400 {
		return containerRole(e) + " " + fallback
	}
	return containerRole(e) + " keeps failing its own readiness " +
		"check (HTTP " + strconv.Itoa(code) + ") while it runs"
}

// resultWords is the probe result of a note in parentheses with a
// leading space, or empty.
func resultWords(note inventory.Note) string {
	if words := probeResult(note.Message); words != "" {
		return " (" + words + ")"
	}
	return ""
}

// probeCheckEvidence is what the container's probe of kind asks
// ("http /readyz :8081") and its timeout, from its spec; nothing when
// the spec is not known.
func probeCheckEvidence(
	e inventory.Entity, kind string,
) []detection.Evidence {
	check := text(e, kube.ProbeAttr(kind, "check"))
	if check == "" {
		return nil
	}
	out := []detection.Evidence{{
		Label: detection.EvidenceProbeCheck, Value: check}}
	if timeout, ok := number(e, kube.ProbeAttr(kind, "timeout")); ok {
		out = append(out, detection.Evidence{
			Label: detection.EvidenceProbeTimeout,
			Value: strconv.Itoa(int(timeout))})
	}
	return out
}
