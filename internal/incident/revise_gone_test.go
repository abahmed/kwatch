package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/abahmed/kwatch/internal/detection"
	"github.com/abahmed/kwatch/internal/detection/reasons"
)

// A revision whose new root was deleted before the update is due says
// nothing: the cause is gone and the resolve tells the rest.
func TestManagerRevisionToDeletedRootSaysNothing(t *testing.T) {
	r := newRig(t, Config{})
	apiPod, _ := workload(r, "api")
	api := sig(apiPod, reasons.CrashLoopBackOff, detection.Warning)
	r.raise(at(0), api)
	require.Len(t, r.tick(at(DefaultSettle)), 1, "api is announced")
	r.observe(cm("cfg-c"))
	r.remove(cm("cfg-c"), DefaultSettle+time.Second)

	ds := moveTo(r, api, tieCause(cm("cfg-c"), 0.9, apiPod))

	wantNone(t, ds)
}
