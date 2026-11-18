package app

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/health"
	"github.com/abahmed/kwatch/internal/kubeclient"
)

// pressureTime is a clock the test moves by hand.
type pressureTime struct{ now time.Time }

func (c *pressureTime) Now() time.Time { return c.now }

// API pressure is a coverage note in /health, never an incident: the
// component is degraded with rate_limited while it lasts and running
// again once the API server lets kwatch through.
func TestReportAPIPressureIsAHealthSignal(t *testing.T) {
	sink := &fakeStatusSink{}
	clock := &pressureTime{now: time.Unix(1e9, 0)}
	pressure := kubeclient.NewPressure(clock)

	reportAPIPressure(sink, pressure)
	require.Equal(t, recordedStatus{"running", "", true},
		sink.statuses[health.APIPressureComponent])

	for range 3 {
		pressure.Note(kubeclient.SignalThrottled)
	}
	reportAPIPressure(sink, pressure)
	require.Equal(t, recordedStatus{"degraded", "rate_limited", false},
		sink.statuses[health.APIPressureComponent])

	clock.now = clock.now.Add(time.Hour)
	reportAPIPressure(sink, pressure)
	require.Equal(t, recordedStatus{"running", "", true},
		sink.statuses[health.APIPressureComponent])

	reportAPIPressure(nil, pressure)
	reportAPIPressure(sink, nil)
}
