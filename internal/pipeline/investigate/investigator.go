package investigate

import (
	"context"
	"time"
	"unicode/utf8"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/redact"
)

// Investigation is one planned read about an incident. Plan builds it on
// the decision loop without I/O; Run does the reads on a pool worker
// under a deadline of Budget.
type Investigation struct {
	// Kind names the investigator, such as "crash" or "node".
	Kind string
	// Budget is how long Run may take. Zero takes MaxBudget.
	Budget time.Duration
	Run    func(context.Context) Result
}

// Result is what an investigation found. Output and Evidence are the
// bounded summary channel messages quote; Report is every section the
// checks read, for the dashboard, and Correlation what kwatch concluded
// across them. Everything is redacted.
type Result struct {
	// Output is the application's own recent output, shown as a quote.
	Output []string
	// Evidence holds the facts writers quote as proof.
	Evidence []incident.Fact
	// Report holds one section per check, in priority order.
	Report []incident.Section
	// Correlation is nil when nothing could be correlated.
	Correlation *incident.Correlation
}

// Investigator plans the investigation of an incident. Plan reports
// false when no check applies to it.
type Investigator interface {
	Plan(p incident.Incident) (Investigation, bool)
}

// ContainerOutput reads a container's recent output, redacted and
// oldest first.
type ContainerOutput func(context.Context, inventory.EntityID) []string

// LogTail reads one run of a container's output for the report; the
// error says why a read failed.
type LogTail func(
	ctx context.Context, container inventory.EntityID, previous bool,
) ([]string, error)

// ServiceEndpoints counts the ready endpoints of a Service. ok is false
// when the count could not be read.
type ServiceEndpoints func(
	ctx context.Context, service inventory.EntityID,
) (ready int, ok bool)

// Sources are what checks read. Model is the live model; it is safe to
// read from pool workers. Every other source does API reads and is
// optional: a missing one leaves its section denied or out.
type Sources struct {
	Model inventory.Reader
	Logs  ContainerOutput
	// CurrentLogs reads only the container's current run, for one that
	// is still running.
	CurrentLogs ContainerOutput
	Endpoints   ServiceEndpoints
	// Tail reads the long log tails of the report.
	Tail LogTail
	// Readyz reads the API server's /readyz?verbose lines.
	Readyz func(context.Context) ([]string, error)
	// PodUsage reads a pod's usage from the metrics API.
	PodUsage func(context.Context, inventory.EntityID) (
		[]kube.ContainerUsage, error)
	// Now stamps when each check returned; nil leaves sections
	// unstamped.
	Now func() time.Time
	// LogsOff is set when log capture is turned off
	// (investigation.logs: false): no log is read at all.
	LogsOff bool
}

// RootInvestigator runs every check that applies to an incident, in
// parallel, under the plan's budget. The first applicable check that
// can lead (the root-kind investigators, in priority order) gives the
// channel summary; every check adds its sections to the report.
type RootInvestigator struct {
	sources Sources
	// slots bounds the API reads in flight across all investigations.
	slots chan struct{}
}

// maxAPIReads bounds the API reads checks make at once, across all
// investigations, so a storm of incidents cannot flood the API server.
const maxAPIReads = 4

// NewInvestigator returns the investigator over sources. With LogsOff
// the log sources are dropped, so no code path can read a log.
func NewInvestigator(sources Sources) *RootInvestigator {
	if sources.LogsOff {
		sources.Logs, sources.CurrentLogs, sources.Tail = nil, nil, nil
	}
	return &RootInvestigator{sources: sources,
		slots: make(chan struct{}, maxAPIReads)}
}

// Plan implements Investigator.
func (r *RootInvestigator) Plan(p incident.Incident) (Investigation, bool) {
	if r.sources.Model == nil {
		return Investigation{}, false
	}
	checks := applicable(p, r.sources.Model)
	if len(checks) == 0 {
		return Investigation{}, false
	}
	lead := leadOf(p, checks)
	kind := checks[0].name
	if lead >= 0 {
		kind = checks[lead].name
	}
	sources := r.gated()
	return Investigation{Kind: kind, Budget: MaxBudget,
		Run: func(ctx context.Context) Result {
			out := runChecks(ctx, sources, p, checks, lead)
			out.Correlation = correlate(sources.Model, p, out.Report)
			return annotateResult(out, sources.Model)
		}}, true
}

// Evidence bounds. A message quotes one or two facts; the rest is noise.
const (
	maxEvidence     = 4
	maxEvidenceText = 200
	maxOutputLines  = 5
)

// Bounded caps a result's size and redacts credentials once more, so a
// buggy check can never leak a secret or flood a message. The summary
// gets the message caps and the report the larger report caps. Private
// addresses stay: they are no secret and help whoever debugs.
func Bounded(r Result) Result {
	out := Result{Report: boundReport(r.Report),
		Correlation: boundCorrelation(r.Correlation)}
	for _, line := range r.Output {
		if len(out.Output) == maxOutputLines {
			break
		}
		out.Output = append(out.Output, clip(redact.Credentials(line)))
	}
	for _, e := range r.Evidence {
		if len(out.Evidence) == maxEvidence {
			break
		}
		e.Text = clip(redact.Credentials(e.Text))
		e.Subject = clip(e.Subject)
		if e.Text != "" {
			out.Evidence = append(out.Evidence, e)
		}
	}
	return out
}

// clip cuts text to maxEvidenceText bytes on a rune boundary.
func clip(text string) string {
	if len(text) <= maxEvidenceText {
		return text
	}
	cut := maxEvidenceText
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return text[:cut] + "…"
}
