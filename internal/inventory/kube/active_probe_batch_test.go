package kube

import (
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/inventory"
)

func TestProbeBatchLimitsProbesInFlight(t *testing.T) {
	const probes = 5 * maxProbeConcurrency
	var batch probeBatch
	var inFlight, peak, started atomic.Int32
	release := make(chan struct{})
	probe := func() inventory.Observation {
		now := inFlight.Add(1)
		for {
			seen := peak.Load()
			if now <= seen || peak.CompareAndSwap(seen, now) {
				break
			}
		}
		started.Add(1)
		<-release
		inFlight.Add(-1)
		return inventory.Observation{}
	}
	produced := make(chan struct{})
	go func() {
		defer close(produced)
		for i := 0; i < probes; i++ {
			batch.run(probe)
		}
	}()

	require.Eventually(t, func() bool {
		return started.Load() == maxProbeConcurrency
	}, 5*time.Second, time.Millisecond)
	select {
	case <-produced:
		t.Fatal("the batch started every probe at once")
	case <-time.After(20 * time.Millisecond):
	}
	assert.EqualValues(t, maxProbeConcurrency, inFlight.Load())

	close(release)
	<-produced
	observations := batch.wait()

	assert.Len(t, observations, probes, "every probe still runs")
	assert.LessOrEqual(t, peak.Load(), int32(maxProbeConcurrency))
}
