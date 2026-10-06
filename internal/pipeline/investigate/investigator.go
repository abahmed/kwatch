package investigate

import (
	"context"
	"time"
	"unicode/utf8"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
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

// Result is what an investigation found. Both parts are redacted.
type Result struct {
	// Output is the application's own recent output, shown as a quote.
	Output []string
	// Evidence holds the facts writers quote as proof.
	Evidence []incident.Fact
}

// Investigator plans the investigation of an incident. Plan reports
// false when no investigator applies to it.
type Investigator interface {
	Plan(p incident.Incident) (Investigation, bool)
}

// ContainerOutput reads a container's recent output, redacted and
// oldest first.
type ContainerOutput func(context.Context, inventory.EntityID) []string

// ServiceEndpoints counts the ready endpoints of a Service. ok is false
// when the count could not be read.
type ServiceEndpoints func(
	ctx context.Context, service inventory.EntityID,
) (ready int, ok bool)

// Sources are what investigators read. Model is the live model; it is
// safe to read from pool workers. Logs and Endpoints do API reads and
// are optional.
type Sources struct {
	Model     inventory.Reader
	Logs      ContainerOutput
	Endpoints ServiceEndpoints
}

// RootInvestigator picks the investigator of an incident by the kind of
// its root: crashes read logs, nodes read conditions and usage, and so
// on (see investigators).
type RootInvestigator struct {
	sources Sources
}

// NewInvestigator returns the root-kind investigator over sources.
func NewInvestigator(sources Sources) *RootInvestigator {
	return &RootInvestigator{sources: sources}
}

// Plan implements Investigator: the first investigator whose kind
// matches p plans the reads.
func (r *RootInvestigator) Plan(p incident.Incident) (Investigation, bool) {
	if r.sources.Model == nil {
		return Investigation{}, false
	}
	for _, inv := range investigators {
		if !inv.matches(p) {
			continue
		}
		sources, read := r.sources, inv.read
		return Investigation{Kind: inv.kind, Budget: inv.budget,
			Run: func(ctx context.Context) Result {
				return annotateResult(read(ctx, sources, p),
					sources.Model)
			}}, true
	}
	return Investigation{}, false
}

// Evidence bounds. A message quotes one or two facts; the rest is noise.
const (
	maxEvidence     = 4
	maxEvidenceText = 200
	maxOutputLines  = 5
)

// Bounded caps a result's size and redacts credentials once more, so a
// buggy investigator can never leak a secret or flood a message.
// Private addresses stay: they are no secret and help whoever debugs.
func Bounded(r Result) Result {
	out := Result{}
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
