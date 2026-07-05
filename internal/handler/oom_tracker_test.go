package handler

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewOomTracker(t *testing.T) {
	ot := newOomTracker(3, 5*time.Minute)
	assert.NotNil(t, ot)
	assert.Equal(t, 3, ot.threshold)
	assert.Equal(t, 5*time.Minute, ot.window)
}

func TestOomTrackerRecordBelowThreshold(t *testing.T) {
	ot := newOomTracker(3, 5*time.Minute)
	now := time.Now()
	ot.now = func() time.Time { return now }

	count, repeating := ot.record("key1")
	assert.Equal(t, 1, count)
	assert.False(t, repeating)
}

func TestOomTrackerRecordMeetsThreshold(t *testing.T) {
	ot := newOomTracker(3, 5*time.Minute)
	now := time.Now()
	ot.now = func() time.Time { return now }

	ot.record("key1")
	ot.record("key1")
	count, repeating := ot.record("key1")
	assert.Equal(t, 3, count)
	assert.True(t, repeating)
}

func TestOomTrackerRecordExpiredEntries(t *testing.T) {
	ot := newOomTracker(2, 5*time.Minute)
	now := time.Now()
	ot.now = func() time.Time { return now }

	ot.record("key1")
	ot.now = func() time.Time { return now.Add(10 * time.Minute) }
	count, repeating := ot.record("key1")
	assert.Equal(t, 1, count)
	assert.False(t, repeating)
}
