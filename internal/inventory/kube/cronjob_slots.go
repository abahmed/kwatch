package kube

import (
	"time"

	robfig "github.com/robfig/cron/v3"
	batchv1 "k8s.io/api/batch/v1"

	"github.com/abahmed/kwatch/internal/inventory"
)

// CronJob schedule attributes, kept so a detector can count the runs
// that were due without parsing the CronJob again.
const (
	// AttrSchedule is spec.schedule.
	AttrSchedule = "schedule.expr"
	// AttrTimeZone is spec.timeZone; absent when unset.
	AttrTimeZone = "schedule.zone"
)

// maxSlotCount bounds how many schedule times ScheduleSlots walks.
const maxSlotCount = 1000

// scheduleText records the schedule and time zone of a CronJob.
func scheduleText(cron *batchv1.CronJob, attrs map[string]inventory.Value) {
	attrs[AttrSchedule] = inventory.Text(
		boundedLabel(cron.Spec.Schedule, maxConditionTypeLength))
	if zone := cron.Spec.TimeZone; zone != nil && *zone != "" {
		attrs[AttrTimeZone] = inventory.Text(
			boundedLabel(*zone, maxConditionTypeLength))
	}
}

// ScheduleSlots counts the schedule times after "after" up to and
// including "until". It reports false for a schedule or zone that does
// not parse. The count stops at maxSlotCount.
func ScheduleSlots(
	expr, zone string, after, until time.Time,
) (int, bool) {
	schedule, err := robfig.ParseStandard(expr)
	if err != nil {
		return 0, false
	}
	if zone != "" {
		location, err := time.LoadLocation(zone)
		if err != nil {
			return 0, false
		}
		after = after.In(location)
	}
	count := 0
	for at := schedule.Next(after); !at.IsZero() && !at.After(until) &&
		count < maxSlotCount; at = schedule.Next(at) {
		count++
	}
	return count, true
}
