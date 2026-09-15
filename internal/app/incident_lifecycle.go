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
		var diagnosis *insight.Insight
		if opts.insightEngine != nil {
			pattern := inc.Reason
			if action != model.ActionResolved {
				diagnosis = opts.diagnose(inc, action)
				if diagnosis != nil && diagnosis.Pattern != "" {
					pattern = diagnosis.Pattern
				}
			}
			opts.insightEngine.ObserveOutcome(inc, action, pattern)
			if opts.saveFeedback != nil &&
				(action == model.ActionCreate || action == model.ActionResolved) {
				opts.saveFeedback()
			}
		}
		if action != model.ActionSkip {
			opts.auditLogger.LogIncident(inc, action)
			if diagnosis == nil && action != model.ActionResolved {
				diagnosis = opts.diagnose(inc, action)
			}
			opts.notify(inc, action, diagnosis)
		}
		metrics.DefaultRegistry().ActiveIncidents.Store(
			int64(holder.engine.ActiveCount()),
		)
	}
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
