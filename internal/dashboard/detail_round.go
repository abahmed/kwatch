package dashboard

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
	"github.com/abahmed/kwatch/internal/redact"
	"github.com/abahmed/kwatch/internal/rootcause"
)

// RoundDetail is what the issue page says beyond the objects: where a
// dependency's address was read, errors in the log of pods Kubernetes
// sees as healthy, failures that began together, the share of the
// failing pods behind a change, the chain of events, the check that
// settled a close call, and whether the cause is still there once the
// issue is resolved. Every part is absent when kwatch has nothing to say.
type RoundDetail struct {
	Config   *RoundConfig   `json:"config,omitempty"`
	Burst    *RoundBurst    `json:"burst,omitempty"`
	Together *RoundTogether `json:"together,omitempty"`
	Blast    *RoundBlast    `json:"blast,omitempty"`
	Chain    []ChainStep    `json:"chain,omitempty"`
	Check    *RoundCheck    `json:"check,omitempty"`
	// Candidates are the stated cause and its close rival, for the
	// Details tab; Pods are the failing pods behind the overlap.
	Candidates []RoundCandidate `json:"candidates,omitempty"`
	// CauseNow is "still" or "recovered" for a resolved issue whose
	// cause kwatch looked at again, else empty.
	CauseNow string `json:"causeNow,omitempty"`
}

// RoundConfig is the ConfigMap key an address was read from.
type RoundConfig struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Key       string `json:"key"`
	Address   string `json:"address"`
	More      int    `json:"more,omitempty"`
}

// RoundBurst is a burst of errors in the log of a workload whose pods
// are Running and Ready: the most frequent line and how often it came
// up now and before, and what Kubernetes reports.
type RoundBurst struct {
	Errors      int    `json:"errors"`
	WindowMin   int    `json:"windowMin"`
	Line        string `json:"line,omitempty"`
	Count       int    `json:"count,omitempty"`
	Before      int    `json:"before"`
	ReadyPods   int    `json:"readyPods,omitempty"`
	DesiredPods int    `json:"desiredPods,omitempty"`
	// PerMinute and Usual are errors per minute now and before.
	PerMinute float64 `json:"perMinute"`
	Usual     float64 `json:"usual"`
}

// RoundTogether is workloads that began failing within seconds of each
// other under a shared upstream.
type RoundTogether struct {
	Workloads int `json:"workloads"`
	Seconds   int `json:"seconds"`
}

// RoundBlast is how many of the failing pods are downstream of the
// changed cause, with the verb for its kind ("read", "use", "mount").
type RoundBlast struct {
	Count int    `json:"count"`
	Total int    `json:"total"`
	Verb  string `json:"verb"`
	// Pods lists each failing pod and whether it uses the object.
	Pods []RoundPod `json:"pods,omitempty"`
}

// ChainStep is one step of the chain of events, in the order each step
// was first seen failing. At is absent when the time is not known.
type ChainStep struct {
	At     time.Time `json:"at,omitzero"`
	Kind   string    `json:"kind"`
	Name   string    `json:"name"`
	Place  string    `json:"namespace,omitempty"`
	Event  string    `json:"event"`
	Number int       `json:"step"`
}

// RoundCheck is the read-only check kwatch ran to tell two close
// causes apart, in its own words, with the two candidates.
type RoundCheck struct {
	Text  string `json:"text"`
	Cause string `json:"cause"`
	Rival string `json:"rival"`
}

// checkedPrefix starts the text of a close-call check fact.
const checkedPrefix = "kwatch checked"

// roundOf gathers the round's parts for p; nil when it has none.
func roundOf(
	model inventory.Reader, p incident.Incident, facts []string,
) *RoundDetail {
	r := &RoundDetail{}
	if cause := p.Cause; cause != nil {
		r.Config = configOf(cause)
		r.Together, r.Blast = proofsOf(cause)
		r.Chain = chainOf(p, cause)
		r.Check = checkOf(cause, facts)
		r.Candidates = candidatesOf(cause, r.Check)
		if r.Blast != nil {
			r.Blast.Pods = blastPodsOf(model, p, r.Blast)
		}
	}
	r.Burst = burstOf(model, p)
	if p.State == incident.Resolved {
		r.CauseNow = causeNow(model, p)
	}
	if r.Config == nil && r.Burst == nil && r.Together == nil &&
		r.Blast == nil && len(r.Chain) == 0 && r.Check == nil &&
		len(r.Candidates) == 0 &&
		r.CauseNow == "" {
		return nil
	}
	return r
}

func configOf(cause *rootcause.CauseRecord) *RoundConfig {
	src := cause.Source
	if src == nil {
		return nil
	}
	return &RoundConfig{Namespace: src.ConfigMap.Namespace,
		Name: src.ConfigMap.Name, Key: src.Key, Address: src.Address,
		More: src.More}
}

func proofsOf(
	cause *rootcause.CauseRecord,
) (*RoundTogether, *RoundBlast) {
	var together *RoundTogether
	var blast *RoundBlast
	for _, item := range cause.Proof {
		if !item.Supports {
			continue
		}
		switch item.Code {
		case rootcause.ProofCoOnset:
			together = &RoundTogether{Workloads: item.Count,
				Seconds: item.Total}
		case rootcause.ProofBlastOverlap:
			if item.Total >= 2 && item.Count*2 >= item.Total {
				blast = &RoundBlast{Count: item.Count, Total: item.Total,
					Verb: blastVerb(cause.Root.Kind)}
			}
		}
	}
	return together, blast
}

// blastVerb is how a pod depends on an object of the kind.
func blastVerb(kind inventory.Kind) string {
	switch kind {
	case kube.KindSecret:
		return "use"
	case kube.KindPVC:
		return "mount"
	}
	return "read"
}

// chainOf is the cause's chain in the order of onsets, three steps or
// more; same-minute steps with the same event keep the chain's order.
func chainOf(p incident.Incident, cause *rootcause.CauseRecord,
) []ChainStep {
	hops := append([]rootcause.Hop(nil), cause.Hops...)
	sort.SliceStable(hops, func(i, j int) bool {
		a, b := hops[i].Began, hops[j].Began
		return !a.IsZero() && !b.IsZero() && a.Before(b)
	})
	var out []ChainStep
	for i, hop := range hops {
		out = append(out, ChainStep{At: hop.Began, Number: i + 1,
			Kind: string(hop.Entity.Kind), Name: hop.Entity.Name,
			Place: hop.Entity.Namespace, Event: eventOf(p, hop.Entity)})
	}
	if len(out) < 3 {
		return nil
	}
	return out
}

// eventOf is the observed event of a hop, from the findings of the
// entity or of its pods; "failing" when none has a word.
func eventOf(p incident.Incident, hop inventory.EntityID) string {
	best, severity := "failing", detection.Severity(0)
	for _, f := range sortedFindings(p) {
		own := f.Entity == hop || (f.Entity.Namespace == hop.Namespace &&
			strings.HasPrefix(f.Entity.Name, hop.Name+"-"))
		mode := f.Mode
		if mode == "" {
			mode = detection.ModeFor(f.Reason)
		}
		word, known := detection.EventWord(mode)
		if own && known && f.Severity >= severity {
			best, severity = word, f.Severity
		}
	}
	return best
}

func sortedFindings(p incident.Incident) []detection.Finding {
	out := make([]detection.Finding, 0, len(p.Members))
	for _, f := range p.Members {
		out = append(out, f)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Entity.String() < out[j].Entity.String()
	})
	return out
}

// checkOf pairs the close-call check, when one ran, with the two
// candidates it separates.
func checkOf(cause *rootcause.CauseRecord, facts []string) *RoundCheck {
	if cause.Rival == nil {
		return nil
	}
	for _, line := range facts {
		if strings.HasPrefix(line, checkedPrefix) {
			return &RoundCheck{Text: redact.Evidence(line),
				Cause: cause.Root.String(),
				Rival: cause.Rival.Root.String()}
		}
	}
	return nil
}

// burstOf reads the log-burst finding of p and what Kubernetes reports
// for its workload.
func burstOf(model inventory.Reader, p incident.Incident) *RoundBurst {
	for _, f := range sortedFindings(p) {
		if f.Reason != "LogErrorBurst" {
			continue
		}
		b := &RoundBurst{
			Errors: evidenceInt(f, detection.EvidenceLogErrors),
			WindowMin: max(evidenceInt(f,
				detection.EvidenceLogWindow)/60, 1),
			Count:  evidenceInt(f, detection.EvidenceLogLineCount),
			Before: evidenceInt(f, detection.EvidenceLogLineBefore),
		}
		for _, e := range f.Evidence {
			if e.Label == detection.EvidenceLogLine {
				b.Line = redact.Evidence(e.Value)
			}
		}
		b.PerMinute, b.Usual = logRates(model, f.Entity, b.Errors,
			evidenceInt(f, detection.EvidenceLogWindow))
		if model != nil {
			b.ReadyPods = int(attrNumber(model, f.Entity,
				kube.AttrReadyReplicas))
			b.DesiredPods = int(attrNumber(model, f.Entity,
				kube.AttrReplicas))
		}
		return b
	}
	return nil
}

func evidenceInt(f detection.Finding, label string) int {
	for _, e := range f.Evidence {
		if e.Label == label {
			n, _ := strconv.Atoi(strings.TrimSpace(e.Value))
			return n
		}
	}
	return 0
}

func attrNumber(
	model inventory.Reader, id inventory.EntityID, name string,
) float64 {
	a, ok := model.Attribute(id, name)
	if !ok {
		return 0
	}
	n, _ := a.Value.AsNumber()
	return n
}

// causeNow looks at a resolved issue's cause again, now.
func causeNow(model inventory.Reader, p incident.Incident) string {
	switch incident.CheckCause(model, p.Cause) {
	case incident.CauseStill:
		return "still"
	case incident.CauseRecovered:
		return "recovered"
	}
	return ""
}
