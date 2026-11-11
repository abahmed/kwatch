package detectors

import (
	"testing"
	"time"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
)

// A Service that selects no pod for a day is clutter, not a failure:
// advice only, never an alert of its own.
func TestUnusedServiceIsAdvice(t *testing.T) {
	m := newTestModel()
	ctx := testDetectorContext(m, t0.Add(DefaultUnusedAfter+time.Hour))
	f, ok := unusedService(ctx, t0)
	if !ok || f.Reason != reasons.ServiceUnused {
		t.Fatalf("no unused finding: %+v", f)
	}
	if !f.Advisory || f.Severity != detection.Info {
		t.Fatalf("an unused Service must be advice: %+v", f)
	}
}
