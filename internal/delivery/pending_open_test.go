package delivery

import (
	"testing"
)

// An opening a newer revision replaces while both wait for a provider
// is settled as lost, as on the queue path: the provider is then told of
// the conversation in full, not with a bare update or resolve.
func TestPendingSupersedeMarksOpenLost(t *testing.T) {
	am := managerWithEntries([]providerEntry{
		{catalogName: "slack", provider: &fakeProvider{}}})
	am.NotifyIncident(revision("a", 1, "a is down"))
	am.NotifyIncident(revision("a", 2, "a is worse"))
	if len(am.pending) != 1 {
		t.Fatalf("pending=%d", len(am.pending))
	}
	if f := am.openFateOf("slack", "a"); f != openLost {
		t.Errorf("open fate after a pending supersede = %v, want "+
			"openLost", f)
	}
}
