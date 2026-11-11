package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/abahmed/kwatch/internal/inventory"
	"github.com/abahmed/kwatch/internal/inventory/kube"
)

// A finding held over a kind kwatch cannot verify says so on its
// incident once the hold has lasted UnverifiedAfter, and no longer once
// an evaluation decides again.
func TestHeldFindingSaysKwatchCannotVerifyIt(t *testing.T) {
	r, start := announcedRig(t)
	id, pod := r.only().ID, podSig("web").Entity
	r.m.NoteHeld(pod, true, start)
	r.m.NoteHeld(pod, true, start.Add(5*time.Minute))

	assert.Empty(t, r.m.Unverified(id, start.Add(UnverifiedAfter-1)))
	assert.Equal(t,
		"kwatch can't verify this right now: reading pod keeps failing",
		r.m.Unverified(id, start.Add(UnverifiedAfter)),
		"timed from the first hold")

	r.m.cfg.Verifiable = func(k inventory.Kind) bool {
		return k != kube.KindPod
	}
	assert.Equal(t, "kwatch can't verify this right now: it can no "+
		"longer read pod (access was lost)",
		r.m.Unverified(id, start.Add(UnverifiedAfter)))

	r.m.NoteHeld(pod, false, start.Add(UnverifiedAfter))
	assert.Empty(t, r.m.Unverified(id, start.Add(time.Hour)))
	assert.Empty(t, r.m.Unverified("gone", start.Add(time.Hour)))
}
