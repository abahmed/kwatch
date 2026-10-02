package kube_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

func TestCronJobSchemaDescribe(t *testing.T) {
	cj := cronJob("cj1")
	schema := kube.CronJobSchema{}
	desc, ok := schema.Describe(cj)

	assert.True(t, ok)
	assert.Equal(t, inventory.Kind("cronjob"), desc.ID.Kind)
	assert.Equal(t, "cj1", desc.ID.Name)

	// Check template hash
	hash, ok := desc.Attributes["template.hash"]
	assert.True(t, ok)
	assert.NotEmpty(t, hash.AsText())

	// Check suspended attribute
	suspended, ok := desc.Attributes["suspended"]
	assert.True(t, ok)
	b, _ := suspended.AsBool()
	assert.False(t, b)
}

func TestCronJobSuspended(t *testing.T) {
	cj := cronJob("cj1")
	suspend := true
	cj.Spec.Suspend = &suspend

	schema := kube.CronJobSchema{}
	desc, ok := schema.Describe(cj)
	assert.True(t, ok)

	suspended, ok := desc.Attributes["suspended"]
	assert.True(t, ok)
	b, _ := suspended.AsBool()
	assert.True(t, b)
}

func TestCronJobScheduleDiff(t *testing.T) {
	old := cronJob("cj1")
	new := cronJob("cj1")
	new.Spec.Schedule = "0 2 * * *"

	schema := kube.CronJobSchema{}
	changes := schema.Diff(old, new)

	found := false
	for _, c := range changes {
		if c.Path == "spec.schedule" {
			found = true
			assert.Equal(t, "0 * * * *", c.Before)
			assert.Equal(t, "0 2 * * *", c.After)
		}
	}
	assert.True(t, found, "schedule change not found")
}

func TestCronJobLastSchedule(t *testing.T) {
	cj := cronJob("cj1")
	cj.Status.LastScheduleTime = timePtr(fixedTime())

	schema := kube.CronJobSchema{}
	desc, ok := schema.Describe(cj)
	assert.True(t, ok)

	lastSched, ok := desc.Attributes["last.schedule"]
	assert.True(t, ok)
	assert.NotNil(t, lastSched)
}

func TestCronJobLastSuccess(t *testing.T) {
	cj := cronJob("cj1")
	cj.Status.LastSuccessfulTime = timePtr(fixedTime())

	schema := kube.CronJobSchema{}
	desc, ok := schema.Describe(cj)
	assert.True(t, ok)

	lastSucc, ok := desc.Attributes["last.success"]
	assert.True(t, ok)
	assert.NotNil(t, lastSucc)
}

func TestHPASchemaDescribe(t *testing.T) {
	h := hpa("hpa1")
	schema := kube.HPASchema{}
	desc, ok := schema.Describe(h)

	assert.True(t, ok)
	assert.Equal(t, inventory.Kind("horizontalpodautoscaler"),
		desc.ID.Kind)
	assert.Equal(t, "hpa1", desc.ID.Name)

	// Check min/max replicas
	minReplicas, ok := desc.Attributes["replicas.min"]
	assert.True(t, ok)
	num, _ := minReplicas.AsNumber()
	assert.Equal(t, 1.0, num)

	maxReplicas, ok := desc.Attributes["replicas.max"]
	assert.True(t, ok)
	num, _ = maxReplicas.AsNumber()
	assert.Equal(t, 10.0, num)

	// Check current/desired replicas
	current, ok := desc.Attributes["replicas.current"]
	assert.True(t, ok)
	num, _ = current.AsNumber()
	assert.Equal(t, 3.0, num)
}

func TestHPAScalesRelation(t *testing.T) {
	h := hpa("hpa1")
	schema := kube.HPASchema{}
	desc, ok := schema.Describe(h)
	assert.True(t, ok)

	scales := desc.Relations[inventory.Scales]
	assert.Len(t, scales, 1)
	assert.Equal(t, inventory.Kind("deployment"),
		scales[0].Kind)
	assert.Equal(t, "deploy", scales[0].Name)
}

func TestHPAMaxReplicasDiff(t *testing.T) {
	old := hpa("hpa1")
	new := hpa("hpa1")
	newMax := int32(20)
	new.Spec.MaxReplicas = newMax

	schema := kube.HPASchema{}
	changes := schema.Diff(old, new)

	found := false
	for _, c := range changes {
		if c.Path == "spec.maxReplicas" {
			found = true
			assert.Equal(t, "10", c.Before)
			assert.Equal(t, "20", c.After)
		}
	}
	assert.True(t, found, "maxReplicas change not found")
}

func TestHPAMinReplicasDiff(t *testing.T) {
	old := hpa("hpa1")
	newMin := int32(2)
	old.Spec.MinReplicas = &newMin

	new := hpa("hpa1")
	newMin2 := int32(5)
	new.Spec.MinReplicas = &newMin2

	schema := kube.HPASchema{}
	changes := schema.Diff(old, new)

	found := false
	for _, c := range changes {
		if c.Path == "spec.minReplicas" {
			found = true
			assert.Equal(t, "2", c.Before)
			assert.Equal(t, "5", c.After)
		}
	}
	assert.True(t, found, "minReplicas change not found")
}

func TestCronJobTemplateDiff(t *testing.T) {
	old := cronJob("cj1")
	new := cronJob("cj1")
	new.Spec.JobTemplate.Spec.Template.Spec.Containers[0].Image =
		"job:2.0"

	schema := kube.CronJobSchema{}
	changes := schema.Diff(old, new)

	found := false
	for _, c := range changes {
		if c.Path == "containers[job].image" {
			found = true
			assert.Equal(t, "job:1.0", c.Before)
			assert.Equal(t, "job:2.0", c.After)
		}
	}
	assert.True(t, found, "image change not found")
}

func TestCronJobScheduleProblem(t *testing.T) {
	badZone := "Mars/Olympus"
	goodZone := "Europe/Paris"
	tt := []struct {
		name     string
		schedule string
		zone     *string
		want     string
	}{
		{"valid", "0 * * * *", nil, ""},
		{"valid_zone", "0 * * * *", &goodZone, ""},
		{"invalid_schedule", "not a schedule", nil, kube.ScheduleInvalid},
		{"unknown_zone", "0 * * * *", &badZone, kube.ScheduleBadZone},
	}
	for _, tc := range tt {
		t.Run(tc.name, func(t *testing.T) {
			cj := cronJob("cj1")
			cj.Spec.Schedule = tc.schedule
			cj.Spec.TimeZone = tc.zone
			desc, ok := kube.CronJobSchema{}.Describe(cj)
			assert.True(t, ok)
			got := desc.Attributes[kube.AttrScheduleProblem].AsText()
			assert.Equal(t, tc.want, got)
		})
	}
}
