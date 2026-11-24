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

// emptyBudget is the finding of a disruption budget the controller found
// without pods age before t0, as the detector raises it after a restart.
func emptyBudget(age time.Duration) detection.Finding {
	s := sig(entity(kube.KindPDB, "search-pdb"), reasons.PdbSelectsNothing,
		detection.Warning)
	s.Since = t0.Add(-age)
	return s
}

// A restarted kwatch that sees a budget that has selected nothing for
// more than a year sends nothing new: the old problem opens no incident.
func TestOldEmptyBudgetAfterRestartSendsNothing(t *testing.T) {
	r := newRig(t, Config{OldProblem: horizon})
	r.raise(at(0), emptyBudget(447*24*time.Hour))

	wantNone(t, r.tick(at(DefaultSettle)))
	assert.Empty(t, r.m.Export())
}

// Without the old-problem horizon it is still never a message of its
// own: it waits for the digest.
func TestEmptyBudgetWaitsForTheDigest(t *testing.T) {
	r := newRig(t, Config{})
	r.raise(at(0), emptyBudget(time.Hour))
	r.tick(at(DefaultSettle))

	require.Len(t, r.m.Export(), 1)
	assert.Equal(t, Digest, r.only().Tier)
}
