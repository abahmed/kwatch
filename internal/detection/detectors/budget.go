package detectors

import (
	"sort"
	"strings"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// multipleBudgetsEvent is the disruption controller's Warning event on a
// pod that more than one PodDisruptionBudget selects. The eviction API
// refuses such pods, so drains stop at them.
const multipleBudgetsEvent = "MultiplePodDisruptionBudgets"

// budgetMisconfiguration reports PodDisruptionBudgets that do not
// protect what they were meant to: the controller cannot compute them
// (DisruptionAllowed=False, SyncFailed), they select no pod, or they
// overlap another budget.
func budgetMisconfiguration(
	ctx detection.Context, e inventory.Entity,
) []detection.Finding {
	var out []detection.Finding
	if f, ok := budgetSyncFailed(ctx, e); ok {
		out = append(out, f)
	}
	if ctx.Model == nil || !ctx.Synced(kube.KindPod) {
		return out
	}
	selected := budgetPods(ctx, e.ID)
	if f, ok := budgetSelectsNothing(ctx, e, selected); ok {
		out = append(out, f)
	}
	if f, ok := budgetOverlap(ctx, e, selected); ok {
		out = append(out, f)
	}
	return out
}

func budgetSyncFailed(
	ctx detection.Context, e inventory.Entity,
) (detection.Finding, bool) {
	status, reason, since := condition(e, "DisruptionAllowed")
	if status != "False" || reason != "SyncFailed" ||
		!sustained(ctx, "pdb-sync-failed", since, DefaultCustomFailing) {
		return detection.Finding{}, false
	}
	return detection.Finding{
		Reason: reasons.PdbSyncFailed, Severity: detection.Warning,
		Health: detection.Failing, Since: since,
		Summary: "Disruption controller cannot evaluate the budget; " +
			"it allows no evictions",
		Evidence: []detection.Evidence{{
			Label: "message", Value: conditionMessage(e, "DisruptionAllowed"),
		}},
	}, true
}

func budgetSelectsNothing(
	ctx detection.Context, e inventory.Entity, selected []inventory.EntityID,
) (detection.Finding, bool) {
	expected, known := number(e, kube.AttrExpectedPods)
	if !known || expected > 0 || len(selected) > 0 ||
		text(e, kube.AttrSelector) == "" {
		return detection.Finding{}, false
	}
	since := valueSince(e, kube.AttrExpectedPods)
	if !sustained(ctx, "pdb-selects-nothing", since, DefaultBudgetBlocked) {
		return detection.Finding{}, false
	}
	return detection.Finding{
		Reason: reasons.PdbSelectsNothing, Severity: detection.Warning,
		Since:   since,
		Summary: "Disruption budget selects no pods; it protects nothing",
		Evidence: []detection.Evidence{{
			Label: "selector", Value: text(e, kube.AttrSelector),
		}},
	}, true
}

// budgetOverlap reports pods this budget shares with another budget of
// the namespace, or that the controller flagged as selected by several.
func budgetOverlap(
	ctx detection.Context, e inventory.Entity, selected []inventory.EntityID,
) (detection.Finding, bool) {
	if len(selected) == 0 {
		return detection.Finding{}, false
	}
	mine := make(map[inventory.EntityID]bool, len(selected))
	for _, pod := range selected {
		mine[pod] = true
	}
	var others []string
	for _, id := range ctx.Model.Entities(kube.KindPDB) {
		if id == e.ID || id.Namespace != e.ID.Namespace {
			continue
		}
		for _, pod := range budgetPods(ctx, id) {
			if mine[pod] {
				others = append(others, id.Name)
				break
			}
		}
	}
	flagged := flaggedPods(ctx, selected)
	if len(others) == 0 && len(flagged) == 0 {
		return detection.Finding{}, false
	}
	sort.Strings(others)
	evidence := make([]detection.Evidence, 0, len(others)+len(flagged))
	for _, name := range others {
		evidence = append(evidence,
			detection.Evidence{Label: "overlapping budget", Value: name})
	}
	evidence = append(evidence, flagged...)
	summary := "Pods are selected by more than one disruption budget; " +
		"evicting them fails"
	if len(others) > 0 {
		summary += " (also " + strings.Join(others, ", ") + ")"
	}
	return detection.Finding{
		Reason: reasons.PdbOverlap, Severity: detection.Warning,
		Health: detection.Failing, Summary: summary, Evidence: evidence,
	}, true
}

// flaggedPods returns evidence of recent MultiplePodDisruptionBudgets
// events on the selected pods.
func flaggedPods(
	ctx detection.Context, pods []inventory.EntityID,
) []detection.Evidence {
	var out []detection.Evidence
	for _, pod := range pods {
		for _, note := range ctx.Model.Notes(
			pod, ctx.Now.Add(-EventWindow),
		) {
			if note.Warning && note.Reason == multipleBudgetsEvent {
				out = append(out, detection.Evidence{
					Label: pod.Name, Value: note.Message,
				})
				break
			}
		}
	}
	return out
}

// budgetPods are the pods a budget's selector matches now.
func budgetPods(
	ctx detection.Context, id inventory.EntityID,
) []inventory.EntityID {
	var out []inventory.EntityID
	for _, link := range kube.Links(ctx.Model, id) {
		if link.Type == inventory.Selects {
			out = append(out, link.To)
		}
	}
	return out
}
