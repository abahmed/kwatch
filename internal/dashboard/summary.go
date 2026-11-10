package dashboard

import (
	"time"

	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// IncidentSummary is the top of the incident page in five lines, built
// by the server so the page does not stitch it from the other parts:
// what is wrong, why, the strongest proof, the first step, who was told
// and where the incident is in its life. Every text is redacted.
type IncidentSummary struct {
	What  string         `json:"what"`
	Why   SummaryWhy     `json:"why"`
	Proof []SummaryProof `json:"proof"`
	Next  *SummaryNext   `json:"next,omitempty"`
	Told  []SummaryTold  `json:"told"`
	Stage SummaryStage   `json:"stage"`
}

// SummaryWhy is the cause in one sentence, empty when kwatch has none.
// Confidence is high, likely or possible, empty without a cause.
type SummaryWhy struct {
	Text       string `json:"text"`
	Confidence string `json:"confidence"`
	CauseKnown bool   `json:"causeKnown"`
}

// SummaryProof is one piece of evidence: Kind is probe, log, exit,
// scheduler, endpoints, change, node or event; Text says it in plain
// words and Detail is the raw value (the log line, the probe message),
// at most maxProofDetail runes. Count and At are absent when unknown.
type SummaryProof struct {
	Kind   string    `json:"kind"`
	Text   string    `json:"text"`
	Detail string    `json:"detail,omitempty"`
	Count  int       `json:"count,omitempty"`
	At     time.Time `json:"at,omitzero"`
}

// SummaryNext is the first fix step; More counts the steps after it.
type SummaryNext struct {
	Text    string `json:"text,omitempty"`
	Command string `json:"command,omitempty"`
	More    int    `json:"more"`
}

// SummaryTold is what one channel holds of the incident. Kind is chat,
// pager, email or other; State is sent, failed or pending for a
// channel, open, closed, failed or pending for a pager. At is the
// latest send, page or close.
type SummaryTold struct {
	Name     string    `json:"name"`
	Provider string    `json:"provider"`
	Kind     string    `json:"kind"`
	State    string    `json:"state"`
	At       time.Time `json:"at,omitzero"`
}

// SummaryStage is where the incident is, for a stepper: confirming
// (kwatch waits to be sure), happening, recovering or resolved, since
// when. Flapping marks a happening incident that keeps coming back and
// StoppedTracking a resolve kwatch made while it still failed.
type SummaryStage struct {
	Step            string    `json:"step"`
	Since           time.Time `json:"since,omitzero"`
	Flapping        bool      `json:"flapping,omitempty"`
	StoppedTracking bool      `json:"stoppedTracking,omitempty"`
}

// Summary bounds: the proof items, a proof's raw value and the cause
// and headline sentences.
const (
	maxProof       = 3
	maxProofDetail = 300
	maxSummaryText = 500
)

// Stage steps.
const (
	stageConfirming = "confirming"
	stageHappening  = "happening"
	stageRecovering = "recovering"
	stageResolved   = "resolved"
)

// summaryOf builds the summary from the detail d, already redacted and
// filled with its context but not yet fitted to the size limit, and
// the incident p.
func summaryOf(d *IncidentDetail, p incident.Incident) *IncidentSummary {
	what, _ := shortText(whatOf(d, p), maxSummaryText)
	return &IncidentSummary{
		What:  what,
		Why:   whyText(d, p),
		Proof: proofOf(d),
		Next:  nextOf(d.FixSteps),
		Told:  toldOf(d),
		Stage: stageOf(d, p),
	}
}

// whyText is the cause sentence and how sure kwatch is of it.
func whyText(d *IncidentDetail, p incident.Incident) SummaryWhy {
	if p.Cause == nil || d.RootCause == "" {
		return SummaryWhy{}
	}
	text, _ := shortText(causeSentence(d, p), maxSummaryText)
	return SummaryWhy{Text: text, Confidence: confidenceOf(p.Cause),
		CauseKnown: true}
}

// confidenceOf words a cause's score in the levels messages use; a
// cause with a rival is never stated as high.
func confidenceOf(c *rootcause.CauseRecord) string {
	switch {
	case c.Score >= rootcause.High && c.Rival == nil:
		return "high"
	case c.Score >= rootcause.Likely:
		return "likely"
	default:
		return "possible"
	}
}

// nextOf is the first step, nil without steps.
func nextOf(steps []FixStep) *SummaryNext {
	if len(steps) == 0 {
		return nil
	}
	return &SummaryNext{Text: steps[0].Text, Command: steps[0].Command,
		More: len(steps) - 1}
}

// stageOf places p on the stepper. Since is the latest transition into
// its state, else the time the state itself records.
func stageOf(d *IncidentDetail, p incident.Incident) SummaryStage {
	out := SummaryStage{StoppedTracking: d.StoppedTracking}
	fallback := p.Opened
	switch p.State {
	case incident.Settling:
		out.Step = stageConfirming
	case incident.Recovering:
		out.Step, fallback = stageRecovering, p.RecoveringSince
	case incident.Resolved:
		out.Step, fallback = stageResolved, p.Resolved
	default:
		out.Step = stageHappening
		out.Flapping = p.State == incident.Flapping
	}
	out.Since = fallback
	state := p.State.String()
	for i := len(d.Transitions) - 1; i >= 0; i-- {
		if t := d.Transitions[i]; t.To == state && !t.At.IsZero() {
			out.Since = t.At
			break
		}
	}
	return out
}
