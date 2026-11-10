package explain

import (
	"regexp"
	"time"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A container whose own output from the run that failed says the
// program stopped at start (an unhandled exception, a panic, an
// invalid or missing setting, a binary that cannot run) was not killed
// by a probe budget or a liveness probe that came after it: those only
// ended a process that had already failed. Such a line rules out the
// probe rows (containerConfigModes), so the cause is the rollout that
// shipped the build, when one did, or the workload on its own, and does
// not change between two evaluations while the line stands. The line
// itself is kept on the record (startupLine) for the message to quote.

// fatalStart matches a line that ends a program at start: a runtime's
// unhandled exception (.NET, Java, Node), a Go or Rust panic, a Python
// traceback or import error, a fatal or critical log level, an invalid
// or missing setting, a binary that cannot run at all, a cloud API that
// refuses the program's identity or finds none (AWS, Google Cloud,
// Azure), and a write to a read-only file system.
var fatalStart = regexp.MustCompile(`(?i)` +
	`\b(unhandled|uncaught)\b.*\b(exception|error|rejection)\b|` +
	`exception in thread "|unhandledpromiserejection|` +
	`^\s*panic:|\bpanicked at\b|^\s*fatal error:|` +
	`(^|[\s\[|])(fatal|critical|crit)(\]|:|\s\|)|` +
	`\blevel=(fatal|critical|crit)\b|` +
	`^traceback \(most recent call last\)|` +
	`\b(modulenotfounderror|importerror|classnotfoundexception|` +
	`noclassdeffounderror)\b|cannot find module|` +
	`\bconfiguration (is )?invalid\b|\binvalid configuration\b|` +
	`\b(must be|is not|was not) (specified|set|provided|configured)\b|` +
	`\b(is|are) required\b|\bmissing (required )?(environment variable|` +
	`env(ironment)? var|setting|config(uration)?|argument|option)\b|` +
	`\bfailed to (start|initiali[sz]e|load config)|` +
	`\bexec format error\b|\bexec: .*: (not found|permission denied)|` +
	`\baccessdenied(exception)?\b|\bis not authorized to perform\b|` +
	`\bauthorizationfailed\b|\bpermission_denied\b|` +
	`\b(nocredentialproviders|webidentityerr)\b|` +
	`\bread-only file system\b`)

// FatalStartLine reports a log line that says the program stopped at
// start. A line that names a dependency it failed to reach is the
// dependency row's (DependencyIn): the dependency is the cause there,
// not the program.
func FatalStartLine(line string) bool {
	if line == "" || !fatalStart.MatchString(line) {
		return false
	}
	_, dependency := DependencyIn(line)
	return !dependency
}

// startRun is how long a run may last and still have stopped at start:
// long enough for a slow start that a liveness probe gave up on.
const startRun = 5 * time.Minute

// stoppedAtStart reports a container whose last run ended within
// startRun of its start. A run that served for hours and then failed
// did not stop at start, whatever its log said early on. Without both
// times it is not ruled out.
func stoppedAtStart(container inventory.Entity) bool {
	started, ok1 := container.Attribute(kube.AttrLastStarted)
	finished, ok2 := container.Attribute(kube.AttrLastFinished)
	if !ok1 || !ok2 {
		return true
	}
	begin, end := started.Value.AsTime(), finished.Value.AsTime()
	if begin.IsZero() || end.IsZero() || end.Before(begin) {
		return true
	}
	return end.Sub(begin) < startRun
}

// startupErrorLine is the line a crashed container's last run stopped
// with, from its termination message or its log, or "" when it ran
// into no fatal error or did not restart after an error exit.
func (v *view) startupErrorLine(
	id inventory.EntityID, container inventory.Entity,
) string {
	if !restartedAfterError(v.s.Model, id) || !stoppedAtStart(container) {
		return ""
	}
	for _, attr := range []string{kube.AttrLastMessage,
		kube.AttrLastErrorLine} {
		if line := attrText(container, attr); FatalStartLine(line) {
			return line
		}
	}
	return ""
}

// startupLine is the fatal line the root's failing containers stopped
// with at start, for the message to quote: the first one, "" when none
// did.
func (s Snapshot) startupLine(c Cause) string {
	for _, id := range sortedFindingIDs(s) {
		if !ownedBy(s, id, c.Root) {
			continue
		}
		containers := []inventory.EntityID{id}
		if id.Kind == kube.KindPod {
			containers = s.Model.Related(id, inventory.PartOf,
				inventory.Incoming)
		}
		for _, cid := range containers {
			if line := s.fatalLineOf(cid); line != "" {
				return line
			}
		}
	}
	return ""
}

// fatalLineOf is the fatal line a container's last run, which ended
// with an error, stopped with, or "".
func (s Snapshot) fatalLineOf(id inventory.EntityID) string {
	e, ok := s.Model.Entity(id)
	if !ok || id.Kind != kube.KindContainer ||
		!restartedAfterError(s.Model, id) {
		return ""
	}
	for _, attr := range []string{kube.AttrLastMessage,
		kube.AttrLastErrorLine} {
		if line := attrText(e, attr); FatalStartLine(line) {
			return line
		}
	}
	return ""
}

// podStoppedAtStart reports a fatal line at start in container id or in
// any other container of its pod: a sidecar's probe settings must not
// take the blame for the app container's crash.
func (v *view) podStoppedAtStart(
	id inventory.EntityID, container inventory.Entity,
) bool {
	if v.startupErrorLine(id, container) != "" {
		return true
	}
	for _, pod := range v.s.Model.Related(id, inventory.PartOf,
		inventory.Outgoing) {
		for _, other := range v.s.Model.Related(pod, inventory.PartOf,
			inventory.Incoming) {
			if other == id {
				continue
			}
			if e, ok := v.s.Model.Entity(other); ok &&
				v.startupErrorLine(other, e) != "" {
				return true
			}
		}
	}
	return false
}
