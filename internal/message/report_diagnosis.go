package message

import (
	"fmt"

	"github.com/abahmed/kwatch/internal/format"
	"github.com/abahmed/kwatch/internal/insight"
	"github.com/abahmed/kwatch/internal/model"
)

func (rb *ReportBuilder) populateDiagnosis(
	r *Report,
	inc *model.Incident,
	ins *insight.Insight,
) {
	d := &DiagnosisSection{
		Hint: dedupeHint(inc.Hint, r.Name, r.Summary.Label, stateMessage(inc)),
	}
	if ins != nil {
		d.Cause = ins.Cause
		d.Pattern = ins.Pattern
		d.NextSteps = append([]string(nil), ins.NextSteps...)
		d.Impact = ins.Impact
		d.Confidence = ins.Confidence
		d.Evidence = append([]string(nil), ins.Evidence...)
		d.CauseState = ins.CauseState
		d.Provisional = ins.Provisional
		d.ReplicaState = ins.ReplicaState
		d.UnknownSummary = ins.UnknownSummary
		d.Maintenance = ins.Maintenance
		d.Flapping = ins.Flapping
		d.LogSignal = ins.LogSignal
		d.Baseline = ins.Baseline
		if ins.Severity != "" {
			r.Severity = string(ins.Severity)
		}
	}
	if d.Impact == "" && len(inc.AffectedServices) > 0 {
		label := "service"
		if len(inc.AffectedServices) > 1 {
			label = "services"
		}
		d.Impact = "affects " + label + " " + format.JoinNames(
			inc.AffectedServices, 4,
		)
	}
	if inc.OwnerUnhealthy && inc.OwnerKind != "" && d.Cause == "" {
		d.Cause = fmt.Sprintf(
			"owning %s is unhealthy — this looks like a rollout, not an "+
				"isolated crash",
			inc.OwnerKind,
		)
		d.Pattern = "rollout_failure"
	}
	memberImpact := affectedMemberImpact(inc.AffectedMembers)
	if memberImpact != "" {
		if d.Impact == "" {
			d.Impact = memberImpact
		} else {
			d.Impact += "; " + memberImpact
		}
	}
	r.Diagnosis = d
}

func affectedMemberImpact(members []model.AffectedResource) string {
	if len(members) == 0 {
		return ""
	}
	active := 0
	for _, member := range members {
		if member.State == model.StateActive {
			active++
		}
	}
	if active == len(members) {
		return fmt.Sprintf("%d related resources are affected", active)
	}
	if active == 0 {
		return "all related symptoms have recovered"
	}
	return fmt.Sprintf(
		"%d/%d related resources remain affected", active, len(members),
	)
}

func stateMessage(inc *model.Incident) string {
	if inc.LastContainerState == nil {
		return ""
	}
	return inc.LastContainerState.Msg
}
