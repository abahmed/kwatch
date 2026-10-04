package delivery

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// A message that names its carrier exists for the audit log; the digest
// or the startup summary tells people about it, so delivery drops it.
func TestManagerDropsCarriedMessages(t *testing.T) {
	clk := newFakeClock()
	provider := newMessageProvider(nil)
	manager := fakeClockManager(t, clk, provider, nil)
	startManager(t, manager)

	carried := revision("held", 1, "🟡 held is at its maximum")
	carried.Carrier = "digest"
	manager.NotifyIncident(carried)
	manager.NotifyIncident(revision("a", 1, "🔴 a is down"))

	assert.Equal(t, "a", provider.receive(t).Key,
		"only the message delivered on its own reaches the provider")
	assert.Empty(t, provider.sent)
}
