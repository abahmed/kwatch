package app

import (
	"github.com/abahmed/kwatch/internal/incident"
	"github.com/abahmed/kwatch/internal/insight"
	"github.com/abahmed/kwatch/internal/metrics"
	"github.com/abahmed/kwatch/internal/model"
)

// lifecycleHook audits and notifies each incident edge unless it was skipped.
// It is the only place a notification is sent from: the engine routes every
// decision through this hook so audit, diagnosis, and delivery stay aligned.
func lifecycleHook(
	opts *engineOptions,
	holder *engineHolder,
) func(*model.Incident, model.IncidentAction) {
	return func(inc *model.Incident, action model.IncidentAction) {
		reevaluation := action == model.ActionReevaluate
		diagnosis, announce := prepareLifecycleInsight(
			opts, inc, action, reevaluation,
		)
		if !announce {
			return
		}
		if reevaluation {
			action = model.ActionUpdate
		}
		if action != model.ActionSkip {
			if diagnosis == nil && action != model.ActionResolved {
				diagnosis = opts.diagnose(inc, action)
			}
			if diagnosis != nil && diagnosis.SuppressReason != "" {
				opts.auditLogger.LogIncidentWithInsight(
					inc, action, diagnosis,
				)
				metrics.DefaultRegistry().InsightRolloutSuppressions.Add(1)
			} else {
				if opts.insightEngine != nil {
					action = opts.insightEngine.DeliveryAction(inc, action)
				}
				opts.auditLogger.LogIncidentWithInsight(inc, action, diagnosis)
				deliveryIncident := inc
				if diagnosis != nil && diagnosis.Severity.Rank() >
					inc.Severity.Rank() {
					deliveryIncident = inc.Clone()
					deliveryIncident.Severity = diagnosis.Severity
				}
				opts.notify(deliveryIncident, action, diagnosis)
				if opts.insightEngine != nil {
					opts.insightEngine.RecordDelivery(inc)
				}
			}
		}
		metrics.DefaultRegistry().ActiveIncidents.Store(
			int64(holder.engine.ActiveCount()),
		)
	}
}

func prepareLifecycleInsight(
	opts *engineOptions,
	inc *model.Incident,
	action model.IncidentAction,
	reevaluation bool,
) (*insight.Insight, bool) {
	if opts.insightEngine == nil {
		return nil, true
	}
	var diagnosis *insight.Insight
	pattern := inc.Reason
	if action != model.ActionResolved {
		diagnosis = opts.diagnose(inc, action)
		if diagnosis != nil && diagnosis.Pattern != "" {
			pattern = diagnosis.Pattern
		}
	}
	if reevaluation && !opts.insightEngine.ShouldAnnounceReevaluation(
		inc, diagnosis,
	) {
		return nil, false
	}
	if !reevaluation {
		opts.insightEngine.ObserveOutcome(inc, action, pattern)
		if opts.saveFeedback != nil &&
			(action == model.ActionCreate || action == model.ActionResolved) {
			opts.saveFeedback()
		}
	}
	return diagnosis, true
}

// diagnose runs insight only for actions where a diagnosis helps. Resolves
// carry no diagnosis, and mass failures already contain their cause hint.
func (o *engineOptions) diagnose(
	inc *model.Incident,
	action model.IncidentAction,
) *insight.Insight {
	if o.insightEngine == nil || action == model.ActionResolved ||
		incident.IsMassFailureKey(inc.Key) {
		return nil
	}
	return o.insightEngine.Analyze(inc)
}
