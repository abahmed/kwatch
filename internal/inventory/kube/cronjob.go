package kube

import (
	"time"

	robfig "github.com/robfig/cron/v3"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	batchv1 "k8s.io/api/batch/v1"

	"github.com/abahmed/kwatch/internal/inventory"
)

// CronJobSchema describes CronJobs.
type CronJobSchema struct{}

// Kind implements Schema.
func (CronJobSchema) Kind() inventory.Kind { return KindCronJob }

// RelationTypes implements Schema.
func (CronJobSchema) RelationTypes() []inventory.RelationType {
	return []inventory.RelationType{inventory.References}
}

// Describe implements Schema.
func (CronJobSchema) Describe(obj any) (Description, bool) {
	cron, ok := obj.(*batchv1.CronJob)
	if !ok {
		return Description{}, false
	}
	attrs := map[string]inventory.Value{
		AttrSuspended: inventory.Bool(
			cron.Spec.Suspend != nil && *cron.Spec.Suspend),
		AttrActive: inventory.Number(float64(len(cron.Status.Active))),
	}
	template := &cron.Spec.JobTemplate.Spec.Template
	attrs[AttrTemplateHash] = inventory.Text(TemplateHash(template))
	if last := cron.Status.LastScheduleTime; last != nil {
		attrs[AttrLastSchedule] = inventory.Time(last.Time)
	}
	if last := cron.Status.LastSuccessfulTime; last != nil {
		attrs[AttrLastSuccess] = inventory.Time(last.Time)
	}
	if next, ok := nextRun(cron); ok {
		attrs[AttrNextRun] = inventory.Time(next)
	}
	if problem := scheduleProblem(cron); problem != "" {
		attrs[AttrScheduleProblem] = inventory.Text(problem)
	}
	cronJobRuns(cron, attrs)
	rel := relations{}
	rel.add(inventory.References,
		templateReferences(cron.Namespace, template)...)
	return Description{
		ID: objectID(KindCronJob, cron), UID: string(cron.UID),
		Attributes: attrs, Relations: rel,
	}, true
}

// Diff implements Schema.
func (CronJobSchema) Diff(old, new any) []inventory.FieldChange {
	before, ok1 := old.(*batchv1.CronJob)
	after, ok2 := new.(*batchv1.CronJob)
	if !ok1 || !ok2 {
		return nil
	}
	fields := templateDiff(&before.Spec.JobTemplate.Spec.Template,
		&after.Spec.JobTemplate.Spec.Template)
	if before.Spec.Schedule != after.Spec.Schedule {
		fields = append(fields, inventory.FieldChange{
			Path: "spec.schedule", Before: before.Spec.Schedule,
			After: after.Spec.Schedule,
		})
	}
	return append(fields, boolChange("spec.suspend",
		before.Spec.Suspend != nil && *before.Spec.Suspend,
		after.Spec.Suspend != nil && *after.Spec.Suspend)...)
}

// HPASchema describes HorizontalPodAutoscalers.
type HPASchema struct{}

// Kind implements Schema.
func (HPASchema) Kind() inventory.Kind { return KindHPA }

// RelationTypes implements Schema.
func (HPASchema) RelationTypes() []inventory.RelationType {
	return []inventory.RelationType{inventory.Scales}
}

// Describe implements Schema.
func (HPASchema) Describe(obj any) (Description, bool) {
	hpa, ok := obj.(*autoscalingv2.HorizontalPodAutoscaler)
	if !ok {
		return Description{}, false
	}
	attrs := map[string]inventory.Value{
		AttrCurrentReplicas: number(hpa.Status.CurrentReplicas),
		AttrDesiredReplicas: number(hpa.Status.DesiredReplicas),
		AttrMaxReplicas:     number(hpa.Spec.MaxReplicas),
	}
	if hpa.Spec.MinReplicas != nil {
		attrs[AttrMinReplicas] = number(*hpa.Spec.MinReplicas)
	}
	conditions := make([]condition, 0, len(hpa.Status.Conditions))
	for _, c := range hpa.Status.Conditions {
		conditions = append(conditions, condition{
			Type: string(c.Type), Status: string(c.Status),
			Reason: c.Reason, Message: c.Message,
			Since: c.LastTransitionTime.Time,
		})
	}
	setConditions(attrs, conditions)
	rel := relations{}
	target := hpa.Spec.ScaleTargetRef
	rel.add(inventory.Scales, EntityFor(
		target.APIVersion, target.Kind, hpa.Namespace, target.Name))
	return Description{
		ID: objectID(KindHPA, hpa), UID: string(hpa.UID),
		Attributes: attrs, Relations: rel,
	}, true
}

// Diff implements Schema.
func (HPASchema) Diff(old, new any) []inventory.FieldChange {
	before, ok1 := old.(*autoscalingv2.HorizontalPodAutoscaler)
	after, ok2 := new.(*autoscalingv2.HorizontalPodAutoscaler)
	if !ok1 || !ok2 {
		return nil
	}
	var fields []inventory.FieldChange
	beforeMin := replicaText(before.Spec.MinReplicas)
	if afterMin := replicaText(after.Spec.MinReplicas); beforeMin != afterMin {
		fields = append(fields, inventory.FieldChange{
			Path: "spec.minReplicas", Before: beforeMin, After: afterMin,
		})
	}
	if before.Spec.MaxReplicas != after.Spec.MaxReplicas {
		fields = append(fields, inventory.FieldChange{
			Path:   "spec.maxReplicas",
			Before: replicaText(&before.Spec.MaxReplicas),
			After:  replicaText(&after.Spec.MaxReplicas),
		})
	}
	return append(fields, hpaTargetChanges(before, after)...)
}

// nextRun is when the CronJob should next start: the first schedule time
// after the last run (or creation), in the CronJob's time zone.
func nextRun(cron *batchv1.CronJob) (time.Time, bool) {
	schedule, err := robfig.ParseStandard(cron.Spec.Schedule)
	if err != nil {
		return time.Time{}, false
	}
	ref := cron.CreationTimestamp.Time
	if last := cron.Status.LastScheduleTime; last != nil {
		ref = last.Time
	}
	if zone := cron.Spec.TimeZone; zone != nil && *zone != "" {
		if location, err := time.LoadLocation(*zone); err == nil {
			ref = ref.In(location)
		}
	}
	return schedule.Next(ref), true
}

// Schedule problem codes match the reasons the CronJob controller uses in
// its events, so users can search for them.
const (
	ScheduleInvalid = "InvalidSchedule"
	ScheduleBadZone = "UnknownTimeZone"
	scheduleAllFine = ""
)

// scheduleProblem reports why a CronJob can never run: a schedule that
// does not parse or a time zone that does not exist.
func scheduleProblem(cron *batchv1.CronJob) string {
	if _, err := robfig.ParseStandard(cron.Spec.Schedule); err != nil {
		return ScheduleInvalid
	}
	if zone := cron.Spec.TimeZone; zone != nil && *zone != "" {
		if _, err := time.LoadLocation(*zone); err != nil {
			return ScheduleBadZone
		}
	}
	return scheduleAllFine
}
