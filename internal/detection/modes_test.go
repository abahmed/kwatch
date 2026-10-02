package detection

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestModesGroupsReasonsBySortedMode(t *testing.T) {
	var oom []string
	var modes []Mode
	for _, m := range Modes() {
		modes = append(modes, m.Mode)
		if m.Mode == "OOMKilled" {
			oom = m.Reasons
		}
	}
	assert.IsIncreasing(t, modes)
	assert.Equal(t, []string{"OOMKILLED", "OOMKilled"}, oom)
}

func TestModeWithinMatchesFinerModesOnly(t *testing.T) {
	assert.True(t, ModeImagePullRegistry.Within(ModeImagePull))
	assert.True(t, ModeImagePull.Within(ModeImagePull))
	assert.False(t, Mode("ImagePullX").Within(ModeImagePull))
	assert.False(t, ModeImagePull.Within(ModeImagePullRegistry))
	assert.Equal(t, Mode("CrashLoop"), Mode("CrashLoop.Panic").Family())
}

func TestKnownModeCoversTablesFamiliesAndConditions(t *testing.T) {
	for _, m := range Modes() {
		assert.True(t, KnownMode(m.Mode), m.Mode)
	}
	for _, family := range []Mode{ModeDisk, ModeExit, ModeFilesystem,
		ModeInodes, ModeLatency, ModeMemory, ModeNetwork, ModeQuota,
		ModeReference, ModeVolume, ModeWebhook} {
		assert.True(t, KnownMode(family), family)
	}
	assert.True(t, KnownMode(ConditionModeOf("Ready")))
	assert.True(t, KnownMode(ModeTerminating))
	assert.False(t, KnownMode("CrashLop"))
}
