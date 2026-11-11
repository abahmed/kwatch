package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestOldProblemDaysIsTheConfiguredHorizon(t *testing.T) {
	m := &Manager{cfg: Config{OldProblem: 90 * 24 * time.Hour}}
	assert.Equal(t, 90, m.OldProblemDays())
	assert.Zero(t, (&Manager{}).OldProblemDays(), "off")
}
