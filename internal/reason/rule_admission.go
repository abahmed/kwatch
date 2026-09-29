package reason

import (
	"strings"

	"github.com/abahmed/kwatch/internal/knowledge"
	"github.com/abahmed/kwatch/internal/knowledge/kube"
	"github.com/abahmed/kwatch/internal/signal"
)

// AdmissionRule blames an admission webhook that the failure names, when
// the webhook is itself failing (no backend, or failing requests).
type AdmissionRule struct{}

// Name implements Rule.
func (AdmissionRule) Name() string { return "admission" }

// Explain implements Rule.
func (AdmissionRule) Explain(q Query, symptom signal.Signal) []Hypothesis {
	text := strings.ToLower(evidenceText(symptom))
	if !strings.Contains(text, "webhook") {
		return nil
	}
	var out []Hypothesis
	for _, kind := range []knowledge.Kind{
		kube.KindValidatingHook, kube.KindMutatingWebhook,
	} {
		for _, hook := range q.Model.Entities(kind) {
			if !strings.Contains(text, strings.ToLower(hook.Name)) {
				continue
			}
			s := newScorer(0.45)
			s.support(0.25, "the error names the webhook")
			hookSignals := q.Signals.Active(hook)
			summary := "admission webhook " + hook.Name + " rejects requests"
			if len(hookSignals) > 0 {
				s.support(0.2, hookSignals[0].Summary)
				summary = "admission webhook " + hook.Name + ": " +
					strings.ToLower(hookSignals[0].Summary[:1]) +
					hookSignals[0].Summary[1:]
			}
			score, points := s.result()
			out = append(out, Hypothesis{
				Root: hook, RootSignals: hookSignals,
				Chain:   []knowledge.EntityID{hook, symptom.Entity},
				Summary: summary, Points: points, Score: score,
			})
		}
	}
	return out
}

// QuotaRule blames an exhausted ResourceQuota for pods a controller cannot
// create in the same namespace.
type QuotaRule struct{}

// Name implements Rule.
func (QuotaRule) Name() string { return "quota" }

// Explain implements Rule.
func (QuotaRule) Explain(q Query, symptom signal.Signal) []Hypothesis {
	if !containsFold(evidenceText(symptom), "exceeded quota") {
		return nil
	}
	var out []Hypothesis
	for _, quota := range q.Model.Entities(kube.KindQuota) {
		if quota.Namespace != symptom.Entity.Namespace {
			continue
		}
		s := newScorer(0.5)
		s.support(0.3, "the error says the quota is exceeded")
		quotaSignals := q.Signals.Active(quota)
		if len(quotaSignals) > 0 {
			s.support(0.15, quotaSignals[0].Summary)
		}
		score, points := s.result()
		out = append(out, Hypothesis{
			Root: quota, RootSignals: quotaSignals,
			Chain:   []knowledge.EntityID{quota, symptom.Entity},
			Summary: "resource quota " + quota.Name + " is used up",
			Points:  points, Score: score,
		})
	}
	return out
}
