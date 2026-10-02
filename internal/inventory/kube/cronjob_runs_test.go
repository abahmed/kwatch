package kube_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func TestCronJobSchemaCountsRunsSinceSuccess(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cron := cronJob("report")
	cron.CreationTimestamp = metav1.NewTime(base)
	cron.Spec.ConcurrencyPolicy = batchv1.ForbidConcurrent
	success := metav1.NewTime(base.Add(time.Hour + time.Minute))
	last := metav1.NewTime(base.Add(4 * time.Hour))
	cron.Status.LastSuccessfulTime = &success
	cron.Status.LastScheduleTime = &last

	desc, ok := kube.CronJobSchema{}.Describe(cron)

	assert.True(t, ok)
	runs, _ := desc.Attributes[kube.AttrRunsSinceSuccess].AsNumber()
	assert.Equal(t, 3.0, runs, "02:00, 03:00 and 04:00 ran after 01:01")
	assert.Equal(t, "Forbid",
		desc.Attributes[kube.AttrConcurrencyPolicy].AsText())
}

func TestCronJobSchemaNoRunsWithoutSchedule(t *testing.T) {
	cron := cronJob("fresh")

	desc, _ := kube.CronJobSchema{}.Describe(cron)

	_, counted := desc.Attributes[kube.AttrRunsSinceSuccess]
	assert.False(t, counted)
	assert.Equal(t, "Allow",
		desc.Attributes[kube.AttrConcurrencyPolicy].AsText())
}

func TestCronJobSchemaZeroRunsAfterSuccess(t *testing.T) {
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cron := cronJob("ok")
	last := metav1.NewTime(base.Add(time.Hour))
	success := metav1.NewTime(base.Add(time.Hour + 30*time.Second))
	cron.Status.LastScheduleTime = &last
	cron.Status.LastSuccessfulTime = &success

	desc, _ := kube.CronJobSchema{}.Describe(cron)

	runs, _ := desc.Attributes[kube.AttrRunsSinceSuccess].AsNumber()
	assert.Zero(t, runs)
}
