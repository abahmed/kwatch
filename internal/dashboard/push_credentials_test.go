package dashboard

import (
	"context"
	"testing"

	"github.com/abahmed/kwatch/internal/clock"
	"github.com/abahmed/kwatch/internal/storage"
)

// pushStateStored runs RunPush once (its first pass, then ctx ends) and
// reports whether it loaded, and so stored, the push state.
func pushStateStored(t *testing.T, s *Server, store *storage.Store) bool {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s.RunPush(ctx, store, nil)
	_, found, err := storage.StateValues[pushState](store).Get(
		pushStateKey)
	if err != nil {
		t.Fatal(err)
	}
	return found
}

// Loading the push state under the empty credentials would drop every
// browser kept under the real ones, so it waits until credentials are
// there (they may arrive after the start, see SetCredentials).
func TestRunPushWaitsForCredentials(t *testing.T) {
	store := openPushStore(t)
	s := NewServer(Options{}, clock.RealClock{})
	if pushStateStored(t, s, store) {
		t.Fatal("the push state was loaded without credentials")
	}

	s.SetCredentials("tok", "pw")

	if !pushStateStored(t, s, store) {
		t.Fatal("the push state was not loaded with credentials")
	}
}
