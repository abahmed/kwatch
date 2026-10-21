package incident

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A page held at notify because a roll-up paged once for the same moment
// is not news: the roll-up told the problem, so the held incident sends
// no "material change" update for the tier it lost a moment later. Any
// other change of it is still news.
func TestHeldCoveredPageSendsNoUpdateForTheHoldItself(t *testing.T) {
	r := newRig(t, Config{})
	r.raise(at(0), dnsDown("dns-1"), dnsDown("dns-2"))
	ds := r.tick(at(DefaultPageSettle))
	require.Len(t, ds, 2)
	held := ds[1].Incident.ID

	r.m.HoldPage(held)

	assert.Equal(t, Notify, r.of(dnsDown("dns-2").Entity).Tier, "held")
	wantNone(t, r.tick(at(DefaultPageSettle+3*time.Second)))
	wantNone(t, r.tick(at(DefaultPageSettle+time.Minute)))
}
