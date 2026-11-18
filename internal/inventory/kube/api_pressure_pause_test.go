package kube_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// While the API server is under pressure the log sample reads nothing:
// it is optional work. It reads again once the pressure is over.
func TestLogSampleRoundWaitsUnderAPIPressure(t *testing.T) {
	h := newSampleHarness(t)
	high := true
	h.round = kube.NewLogSampleRound(kube.LogSampleConfig{
		Logs: h.reader, Model: h.model, Now: func() time.Time { return h.now },
		Submit: func(context.Context, ...inventory.Observation) {},
		Paused: func() bool { return high }})
	h.workload("orders", true)

	h.round.Round(context.Background())
	assert.Empty(t, h.reader.reads, "paused under pressure")

	high = false
	h.round.Round(context.Background())
	assert.Len(t, h.reader.reads, 1, "reads again once it is over")
}
