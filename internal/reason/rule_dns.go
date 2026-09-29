package reason

import (
	"regexp"

	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
	"github.com/abahmed/kwatch/internal/signal"
)

// dnsFailure matches name-resolution errors from common runtimes.
var dnsFailure = regexp.MustCompile(`(?i)no such host|temporary failure ` +
	`in name resolution|name or service not known|could not resolve ` +
	`host|server misbehaving|lookup .*: i/o timeout`)

// DNSRule blames failing cluster DNS for workloads whose errors are
// name-resolution failures.
type DNSRule struct{}

// Name implements Rule.
func (DNSRule) Name() string { return "cluster-dns" }

// Explain implements Rule.
func (DNSRule) Explain(q Query, symptom signal.Signal) []Hypothesis {
	if !dnsFailure.MatchString(evidenceText(symptom)) {
		return nil
	}
	dnsSignals := q.Signals.Active(kube.ClusterDNS)
	s := newScorer(0.3)
	s.support(0.2, "the error is a name-resolution failure")
	summary := "name resolution fails"
	if len(dnsSignals) > 0 {
		s.support(0.35, "kwatch's own DNS probe fails too")
		summary = "cluster DNS is failing"
	} else {
		s.contradict(0.1, "kwatch's own DNS probe succeeds")
	}
	score, points := s.result()
	return []Hypothesis{{
		Root: kube.ClusterDNS, RootSignals: dnsSignals,
		Chain:   []knowledge.EntityID{kube.ClusterDNS, symptom.Entity},
		Summary: summary, Points: points, Score: score,
	}}
}
