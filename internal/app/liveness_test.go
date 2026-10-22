package app

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestPipelineLivenessFailsWhenTheLoopHangs(t *testing.T) {
	start := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	now := start
	progress := newComponentProgress(start)
	check := pipelineLiveness(progress, func() time.Time { return now })

	now = start.Add(pipelineLivenessStall)
	assert.NoError(t, check(), "a slow start within the grace is alive")

	now = start.Add(pipelineLivenessStall + time.Second)
	assert.Error(t, check(), "no progress past the limit is a hang")

	progress.Touch(now)
	assert.NoError(t, check(), "progress makes it alive again")
}
