package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// While a restored incident's findings come back inside the grace, its
// tier may move with the partial view: digest, notification, digest
// again within seconds. None of that is a tier change of the incident:
// the journal records no rise and no fall.
func TestManagerRestoredTierFlipInsideGraceIsNotRecorded(t *testing.T) {
	hpa := entity(kube.KindHPA, "api")
	failing := sig(hpa, reasons.FailedGetScale, detection.Warning)
	quiet := sig(hpa, reasons.HPATargetMissing, detection.Info)
	src := newRig(t, Config{})
	announced(t, src, failing)
	now := at(2 * time.Minute)
	src.raise(now, quiet)
	src.clear(now, failing)
	src.tick(now)
	src.tick(now.Add(DefaultReviseSettle))
	require.Equal(t, Digest, src.only().Tier)

	dst := newRig(t, Config{})
	start := at(time.Hour)
	dst.m.Restore(src.m.Export(), start.Add(10*time.Minute))
	dst.raise(start.Add(time.Second), failing)
	dst.tick(start.Add(2 * time.Second))
	dst.raise(start.Add(4*time.Second), quiet)
	dst.clear(start.Add(4*time.Second), failing)
	for d := 5 * time.Second; d <= 20*time.Minute; d += 30 * time.Second {
		dst.tick(start.Add(d))
	}

	got := dst.only()
	assert.Equal(t, Digest, got.Tier)
	for _, tr := range got.Journal.Transitions {
		if !tr.At.Before(start) {
			assert.NotEqual(t, TransitionTierRaised, tr.Kind)
			assert.NotEqual(t, TransitionTierLowered, tr.Kind)
		}
	}
}
