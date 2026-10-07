package kube

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestScheduleSlotsCountsDueRuns(t *testing.T) {
	last := time.Date(2024, 1, 1, 2, 0, 0, 0, time.UTC)

	n, ok := ScheduleSlots("0 2 * * *", "", last, last.Add(5*24*time.Hour))
	assert.True(t, ok)
	assert.Equal(t, 5, n)

	n, _ = ScheduleSlots("0 2 * * *", "", last, last.Add(time.Hour))
	assert.Zero(t, n, "the run at 'after' itself is not counted")
}

func TestScheduleSlotsRejectsBadInput(t *testing.T) {
	now := time.Now()
	_, ok := ScheduleSlots("not a cron", "", now, now.Add(time.Hour))
	assert.False(t, ok)
	_, ok = ScheduleSlots("* * * * *", "Mars/Base", now, now.Add(time.Hour))
	assert.False(t, ok)
}
