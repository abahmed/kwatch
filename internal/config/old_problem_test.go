package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestOldProblemDaysDefaultsToThirtyDays(t *testing.T) {
	runtime := RuntimeConfigFor(DefaultConfig())
	assert.Equal(t, 30*24*time.Hour, runtime.Lifecycle().OldProblem())

	off := DefaultConfig()
	off.OldProblemDays = 0
	assert.Zero(t, CompileRuntimeConfig(off).Lifecycle().OldProblem())
	assert.Empty(t, validateLifecycleSettings(off))
}

func TestOldProblemDaysRejectsANegativeValue(t *testing.T) {
	cfg := DefaultConfig()
	cfg.OldProblemDays = -1
	errs := validateLifecycleSettings(cfg)
	if assert.Len(t, errs, 1) {
		assert.Contains(t, errs[0].Error(), "oldProblemDays")
	}
}

// A huge oldProblemDays used to overflow the duration: past 106,751
// days it went negative (the setting off), and 213,504 came out as 25
// minutes. It is clamped to a century, and still accepted.
func TestOldProblemDaysIsClampedNotRejected(t *testing.T) {
	century := 36500 * 24 * time.Hour
	for _, days := range []int{36500, 36501, 106752, 213504} {
		cfg := DefaultConfig()
		cfg.OldProblemDays = days
		assert.Empty(t, validateLifecycleSettings(cfg), "%d days", days)
		assert.Equal(t, century,
			CompileRuntimeConfig(cfg).Lifecycle().OldProblem(),
			"%d days", days)
	}
}
