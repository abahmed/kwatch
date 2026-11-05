package dashboard

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/health"
)

func TestKwatchStatusServesTheResources(t *testing.T) {
	s := newTestServer()
	s.SetSelf(func() (health.SelfReport, bool) {
		return health.SelfReport{StateFileBytes: 4096}, true
	})
	s.SetResources(func() health.ResourceUse {
		return health.ResourceUse{MemoryKnown: true, MemoryBytes: 100,
			MemoryLimit: 200, CPUKnown: true, CPUCores: 0.04,
			CPULimit: 0.5, DiskKnown: true, DiskUsed: 10, DiskTotal: 40}
	})

	got, body := readKwatch(t, s)

	require.NotNil(t, got.Resources)
	assert.Equal(t, ResourcesStatus{
		Memory:    &MemoryUse{Used: 100, Limit: 200},
		CPU:       &CPUUse{Cores: 0.04, LimitCores: 0.5},
		Disk:      &DiskUse{Used: 10, Total: 40},
		StateFile: &StateFileStatus{Bytes: 4096},
	}, *got.Resources)
	assert.Contains(t, body, `"limitCores":0.5`)
}

func TestKwatchStatusResourcesLeaveOutWhatIsUnknown(t *testing.T) {
	s := newTestServer()
	s.SetResources(func() health.ResourceUse {
		// No session: the disk has no state file to stand beside.
		return health.ResourceUse{MemoryKnown: true, MemoryBytes: 100,
			DiskKnown: true, DiskUsed: 10, DiskTotal: 40}
	})

	got, body := readKwatch(t, s)

	require.NotNil(t, got.Resources)
	assert.Equal(t, ResourcesStatus{Memory: &MemoryUse{Used: 100}},
		*got.Resources, "no limit, no CPU rate yet, no disk")
	assert.NotContains(t, body, `"limit"`)

	s.SetResources(func() health.ResourceUse { return health.ResourceUse{} })
	got, body = readKwatch(t, s)
	assert.Nil(t, got.Resources)
	assert.NotContains(t, body, "resources")

	s.SetResources(nil)
	got, _ = readKwatch(t, s)
	assert.Nil(t, got.Resources)
}
